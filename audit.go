package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/syslog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	configFile = flag.String("config", "", "Config file location")
)

func main() {
	flag.Parse()

	logrus.SetLevel(logrus.InfoLevel)

	if *configFile == "" {
		logrus.Error("a config file must be provided")
		flag.Usage()
	}

	config, err := loadConfig(*configFile)
	if err != nil {
		logrus.WithError(err).Fatal("failed to load configuration")
	}

	if err := initWebServer(config.MetricsAddress); err != nil {
		logrus.WithError(err).Fatal("failed to init metrics")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// output needs to be created before anything that write to stdout
	writer, err := createOutput(ctx, config)
	if err != nil {
		logrus.WithError(err).Fatal("failed to create output")
	}

	if err := setRules(config, exe); err != nil {
		logrus.WithError(err).Fatal("failed to set rules")
	}

	nlClient, err := NewNetlinkClient(config.SockerBuffer.Receive)
	if err != nil {
		logrus.WithError(err).Fatal("failed to create netlink client")
	}
	filter, err := createFilters(config)
	if err != nil {
		logrus.WithError(err).Fatal("failed to create filters")
	}
	marshaller := NewAuditMarshaller(
		writer,
		uint16(config.Events.Min),
		uint16(config.Events.Max),
		config.MessageTracking.Enabled,
		config.MessageTracking.LogOutOfOrder,
		config.MessageTracking.MaxOutOfOrder,
		filter,
	)

	logrus.Infof("started processing events in the range [%d, %d]", config.Events.Min, config.Events.Max)

	go loop(ctx, nlClient, marshaller)

	stop := make(chan os.Signal, 1)
	go func() {
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		defer signal.Stop(stop)
	}()

	<-stop
	cancel()
}

func loop(ctx context.Context, nlClient *NetlinkClient, marshaller *AuditMarshaller) {
	//Main loop. Get data from netlink and send it to the json lib for processing
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := nlClient.Receive()
			if err != nil {
				logrus.WithError(err).Error("failed to receive a message")
				continue
			}
			if msg == nil {
				continue
			}
			marshaller.Consume(msg)
		}
	}
}

func initWebServer(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", prometheus.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			logrus.WithError(err).WithField("addr", addr).Fatal("failed to start web server")
		}
	}()
	return nil
}

func setRules(config *Config, e executor) error {
	// Clear existing rules
	if err := e("auditctl", "-D"); err != nil {
		return fmt.Errorf("failed to flush existing audit rules: %v", err)
	}

	logrus.Info("flushed existing audit rules")

	// Add ours in
	if rules := config.Rules; len(rules) != 0 {
		for i, v := range rules {
			// Skip rules with no content
			if v == "" {
				continue
			}

			if err := e("auditctl", strings.Fields(v)...); err != nil {
				return fmt.Errorf("failed to add rule #%d: %v", i+1, err)
			}

			logrus.Infof("added audit rule #%d", i+1)
		}
	} else {
		return errors.New("no audit rules found")
	}

	return nil
}

func createOutput(ctx context.Context, config *Config) (*AuditWriter, error) {
	var writer *AuditWriter
	var err error
	i := 0

	if config.Output.Syslog.Enabled {
		i++
		writer, err = createSyslogOutput(config)
		if err != nil {
			return nil, err
		}
	}

	if config.Output.File.Enabled {
		i++
		writer, err = createFileOutput(config)
		if err != nil {
			return nil, err
		}

		go handleLogRotation(config, writer)
	}

	if config.Output.Stdout.Enabled {
		i++
		writer, err = createStdOutOutput(config)
		if err != nil {
			return nil, err
		}
	}

	if config.Output.Kafka.Enabled {
		i++
		writer, err = createKafkaOutput(ctx, config)
		if err != nil {
			return nil, err
		}
	}

	if i > 1 {
		return nil, errors.New("only one output can be enabled at a time")
	}

	if writer == nil {
		return nil, errors.New("no outputs were configured")
	}

	return writer, nil
}

func createSyslogOutput(config *Config) (*AuditWriter, error) {
	attempts := config.Output.Syslog.Attempts
	if attempts < 1 {
		return nil, fmt.Errorf("output attempts for syslog must be at least 1, %v provided", attempts)
	}

	syslogWriter, err := syslog.Dial(
		config.Output.Syslog.Network,
		config.Output.Syslog.Address,
		syslog.Priority(config.Output.Syslog.Priority),
		config.Output.Syslog.Tag,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to open syslog writer: %v", err)
	}

	return NewAuditWriter(syslogWriter, attempts), nil
}

func createFileOutput(config *Config) (*AuditWriter, error) {
	attempts := config.Output.File.Attempts
	if attempts < 1 {
		return nil, fmt.Errorf("output attempts for file must be at least 1, %v provided", attempts)
	}

	mode := os.FileMode(config.Output.File.Mode)
	if mode < 1 {
		return nil, errors.New("output file mode should be greater than 0000")
	}

	f, err := os.OpenFile(
		config.Output.File.Path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, mode,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %v", err)
	}

	if err := f.Chmod(mode); err != nil {
		return nil, fmt.Errorf("failed to set file permissions: %v", err)
	}

	uname := config.Output.File.User
	u, err := user.Lookup(uname)
	if err != nil {
		return nil, fmt.Errorf("could not find uid for user %s: %v", uname, err)
	}

	gname := config.Output.File.Group
	g, err := user.LookupGroup(gname)
	if err != nil {
		return nil, fmt.Errorf("could not find gid for group %s: %v", gname, err)
	}

	uid, err := strconv.ParseInt(u.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("found uid could not be parsed: %v", err)
	}

	gid, err := strconv.ParseInt(g.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("found gid could not be parsed: %v", err)
	}

	if err = f.Chown(int(uid), int(gid)); err != nil {
		return nil, fmt.Errorf("could not chown output file: %v", err)
	}

	return NewAuditWriter(f, attempts), nil
}

func handleLogRotation(config *Config, writer *AuditWriter) {
	// Re-open our log file. This is triggered by a USR1 signal and is meant to be used upon log rotation
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGUSR1)

	for range sigc {
		newWriter, err := createFileOutput(config)
		if err != nil {
			logrus.WithError(err).Fatal("error re-opening log file")
		}

		oldFile := writer.w.(*os.File)
		writer.w = newWriter.w
		writer.e = newWriter.e

		err = oldFile.Close()
		if err != nil {
			logrus.WithError(err).Error("error closing old log file")
		}
	}
}

func createStdOutOutput(config *Config) (*AuditWriter, error) {
	attempts := config.Output.Stdout.Attempts
	if attempts < 1 {
		return nil, fmt.Errorf("output attempts for stdout must be at least 1, %v provided", attempts)
	}

	return NewAuditWriter(os.Stdout, attempts), nil
}

func createKafkaOutput(ctx context.Context, config *Config) (*AuditWriter, error) {
	attempts := config.Output.Kafka.Attempts
	if attempts < 1 {
		return nil, fmt.Errorf("output attempts for Kafka must be at least 1, %v provided", attempts)
	}
	kw, err := NewKafkaWriter(
		ctx,
		config.Output.Kafka,
	)
	if err != nil {
		return nil, err
	}
	return NewAuditWriter(kw, attempts), nil
}

func createFilters(config *Config) ([]AuditFilter, error) {
	var (
		err     error
		filters []AuditFilter
	)
	fs := config.Filters
	if fs == nil {
		return filters
	}

	for i, f := range fs {
		var af AuditFilter
		if f.MessageType != 0 {
			af.messageType = uint16(f.MessageType)
		}
		if f.Regex != "" {
			if af.regex, err = regexp.Compile(f.Regex); err != nil {
				return nil, fmt.Errorf("`regex` in filter %d could not be parsed: %s", i+1, f.Regex)
			}
		}
		if f.Syscall != 0 {
			af.syscall = strconv.Itoa(f.Syscall)
		}

		if af.regex == nil {
			return nil, fmt.Errorf("Filter %d is missing the `regex` entry", i+1)
		}

		if af.syscall == "" {
			return nil, fmt.Errorf("Filter %d is missing the `syscall` entry", i+1)
		}

		if af.messageType == 0 {
			return nil, fmt.Errorf("Filter %d is missing the `message_type` entry", i+1)
		}

		filters = append(filters, af)
		logrus.Infof("Ignoring  syscall `%v` containing message type `%v` matching string `%s`", af.syscall, af.messageType, af.regex.String())
	}

	return filters, nil
}

type executor func(s string, a ...string) error

func exe(s string, a ...string) error {
	return exec.Command(s, a...).Run()
}
