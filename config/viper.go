package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

var Conf *Config

type Config struct {
	Eviction *Eviction `yaml:"eviction"`
}

type Eviction struct {
	CleanUpInterval time.Duration `mapstructure:"clean_up_interval"`
	TTL             time.Duration `mapstructure:"ttl"`
	NumSegments     int           `mapstructure:"num_segments"`
}

var once sync.Once

func init() {
	once.Do(func() {
		fmt.Println("Loading configuration...")
		if err := NewViperConfig(); err != nil {
			panic(err)
		}
	})
}

func NewViperConfig() (err error) {
	relPath, err := getRelativePathFromCaller()
	if err != nil {
		return
	}
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(relPath)
	viper.EnvKeyReplacer(strings.NewReplacer("_", "-"))
	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	return viper.Unmarshal(&Conf)
}

func getRelativePathFromCaller() (relPath string, err error) {
	callerPwd, err := os.Getwd()
	if err != nil {
		return
	}
	_, here, _, _ := runtime.Caller(0)
	relPath, err = filepath.Rel(callerPwd, filepath.Dir(here))
	fmt.Printf("caller from: %s, here: %s, relPath: %s\n", callerPwd, here, relPath)
	return
}
