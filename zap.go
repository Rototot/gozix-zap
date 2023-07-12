// Copyright 2018 Sergey Novichkov. All rights reserved.
// For the full copyright and license information, please view the LICENSE
// file that was distributed with this source code.

package zap

import (
	"fmt"
	"os"
	"strings"

	"github.com/gozix/di"
	gzGlue "github.com/gozix/glue/v3"
	gzViper "github.com/gozix/viper/v3"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// BundleName is default definition name.
const BundleName = "zap"

type (
	// Bundle implements the glue.Bundle interface.
	Bundle struct{}

	// fieldsConf is logger conf fieldsConf struct.
	fieldsConf []struct {
		Key, Value string
	}

	// multiErr is a stub for recognizing then error is a instance of uber-go/multierr
	multiErr interface {
		Errors() []error
	}
)

// Bundle implements glue.Bundle.
var _ gzGlue.Bundle = (*Bundle)(nil)

// NewBundle create bundle instance.
func NewBundle() *Bundle {
	return new(Bundle)
}

func (b *Bundle) Name() string {
	return BundleName
}

// Build implements the glue.Bundle interface.
func (b *Bundle) Build(builder di.Builder) (err error) {
	return builder.Apply(
		di.Provide(b.provideZapLogger, di.Constraint(1, withCoreFactory())),
		di.Provide(b.provideCoreFactory, AsCoreFactory()),
		di.Provide(NewGelfStreamZapFactory, AsCoreFactory()),
	)
}

func (b *Bundle) DependsOn() []string {
	return []string{
		gzViper.BundleName,
	}
}

func (b *Bundle) provideCoreFactory() CoreFactory {
	return &coreFactory{}
}

func (b *Bundle) provideZapLogger(
	cfg *viper.Viper,
	factories []CoreFactory,
) (_ *zap.Logger, _ func() error, err error) {
	var cores []zapcore.Core
	if cores, err = b.cores(cfg, factories); err != nil {
		return nil, nil, err
	}

	var options []zap.Option
	if options, err = b.options(cfg); err != nil {
		return nil, nil, err
	}

	var logger = zap.New(zapcore.NewTee(cores...), options...)
	zap.ReplaceGlobals(logger)
	zap.RedirectStdLog(logger)

	return logger, func() error {
		// os.Stdout.Sync() fails on different consoles. Ignoring error.
		var err = logger.Sync()
		if e, ok := err.(multiErr); ok {
			for _, ee := range e.Errors() {
				if b.handleError(ee) != nil {
					return err
				}
			}

			return nil
		}

		return b.handleError(err)
	}, nil
}

func (b *Bundle) cores(cfg *viper.Viper, factories []CoreFactory) (_ []zapcore.Core, err error) {
	var (
		defs  = cfg.GetStringMap(BundleName + ".cores")
		cores = make([]zapcore.Core, 0, 4)
	)

	var factoriesMap = make(map[string]CoreFactory, len(factories))
	for _, factory := range factories {
		factoriesMap[factory.Name()] = factory
	}

	for def := range defs {
		var path = fmt.Sprintf("%s.cores.%s", BundleName, def)

		if !cfg.IsSet(path + ".type") {
			return nil, fmt.Errorf(`core "%s" should contains type property`, def)
		}

		var coreType = cfg.GetString(path + ".type")
		if _, ok := factoriesMap[coreType]; !ok {
			return nil, fmt.Errorf(`core factory with type "%s" is not defined`, coreType)
		}

		var c zapcore.Core
		if c, err = factoriesMap[coreType].New(cfg, path); err != nil {
			return nil, err
		}

		cores = append(cores, c)
	}

	if len(cores) == 0 {
		cores = append(cores, zapcore.NewNopCore())
	}

	return cores, err
}

// options is options getter.
func (b *Bundle) options(cfg *viper.Viper) (_ []zap.Option, err error) {
	var (
		dev     = cfg.GetBool(BundleName + ".development")
		options = make([]zap.Option, 0, 8)
	)

	if dev {
		options = append(options, zap.Development())
	}

	if cfg.GetBool(BundleName + ".caller") {
		options = append(options, zap.AddCaller())
	}

	if cfg.IsSet(BundleName + ".stacktrace") {
		var level = zap.NewAtomicLevel()
		if err = level.UnmarshalText([]byte(cfg.GetString(BundleName + ".stacktrace"))); err != nil {
			return nil, err
		}

		options = append(options, zap.AddStacktrace(level))
	}

	var fConf = make(fieldsConf, 0, 4)
	if err = cfg.UnmarshalKey(BundleName+".fields", &fConf); err != nil {
		return nil, err
	}

	if len(fConf) > 0 {
		var fields = make([]zapcore.Field, 0, len(fConf))
		for _, f := range fConf {
			fields = append(fields, zap.String(f.Key, f.Value))
		}

		options = append(options, zap.Fields(fields...))
	}

	return options, nil
}

func (b *Bundle) handleError(err error) error {
	if e, ok := err.(*os.PathError); ok {
		if strings.HasPrefix(e.Path, "/dev/std") {
			return nil
		}
	}

	return err
}
