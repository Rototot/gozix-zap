package zap

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ CoreFactory = (*GelfStreamZapFactory)(nil)

// GelfStreamZapFactory implemented GELF specification https://go2docs.graylog.org/5-1/getting_in_log_data/gelf.html
/*
Example for yaml configuration
zap:
	  cores:
		json:
		  type: "gelf_stream"
		  level: "debug"
		  encoding: "json"
	  caller: true
	  fields:
		- key: "service"
		  value: "example-service-name"
	  stacktrace: "error"
	  development: true
*/
type GelfStreamZapFactory struct {
}

func NewGelfStreamZapFactory() *GelfStreamZapFactory {
	return &GelfStreamZapFactory{}
}

func (g GelfStreamZapFactory) Name() string {
	return "gelf_stream"
}

func (g GelfStreamZapFactory) New(conf *viper.Viper, path string) (zapcore.Core, error) {
	var rootPath = strings.Split(path, ".")[0]
	var loggerConf zap.Config
	var key string
	{
		{
			key = rootPath + ".development"
			if conf.IsSet(key) && conf.GetBool(key) {
				loggerConf = zap.NewDevelopmentConfig()
			} else {
				loggerConf = zap.NewProductionConfig()
			}
		}

		loggerConf.Encoding = "json"
		loggerConf.EncoderConfig = getGelfEncoderConfig()
		{
			key = path + ".encoding"
			if conf.IsSet(key) {
				loggerConf.Encoding = conf.GetString(key)
			}

			loggerConf.EncoderConfig.EncodeLevel = gelfLvlEncoder
		}

		key = rootPath + ".stacktrace"
		if conf.IsSet(key) {
			loggerConf.DisableStacktrace = conf.GetString(key) == "false"
		}

		key = rootPath + ".caller"
		if conf.IsSet(key) {
			loggerConf.DisableCaller = !conf.GetBool(key)
		}

		key = path + ".level"
		if conf.IsSet(key) {
			lvl, err := zap.ParseAtomicLevel(conf.GetString(key))
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("cannot parse log level %s", conf.GetString(key)))
			}
			loggerConf.Level = lvl
		}

		// version field
		if conf.IsSet("app.version") {
			loggerConf.InitialFields["version"] = conf.GetString("app.version")
		}
	}

	var core zapcore.Core
	{
		logger, err := loggerConf.Build([]zap.Option{}...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build logger")
		}

		core = logger.Core()
	}

	return core, nil
}

func getGelfEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "_logger",
		CallerKey:      "_caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "short_message",
		StacktraceKey:  "full_message",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.EpochTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

type gelfLevel int

const (
	EmergencyGelfLvl gelfLevel = iota
	AlertGelfLvl
	CriticalGelfLvl
	ErrorGelfLvl
	WarningGelfLvl
	NoticeGelfLvl
	InformationalGelfLvl
	DebugGelfLvl
)

var lvlRelations = map[zapcore.Level]gelfLevel{
	zapcore.DebugLevel:  DebugGelfLvl,
	zapcore.InfoLevel:   InformationalGelfLvl,
	zapcore.WarnLevel:   WarningGelfLvl,
	zapcore.ErrorLevel:  ErrorGelfLvl,
	zapcore.DPanicLevel: EmergencyGelfLvl,
	zapcore.PanicLevel:  EmergencyGelfLvl,
	zapcore.FatalLevel:  EmergencyGelfLvl,
}
var gelfLvlEncoder zapcore.LevelEncoder = func(level zapcore.Level, encoder zapcore.PrimitiveArrayEncoder) {
	encoder.AppendInt(int(lvlRelations[level]))
}
