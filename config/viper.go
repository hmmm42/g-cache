package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var Conf *Config
var DefaultEtcdConfig clientv3.Config

type Config struct {
	Eviction     *Eviction     `mapstructure:"eviction"`
	SingleFlight *SingleFlight `mapstructure:"single_flight"`
	GroupManager *GroupManager `mapstructure:"group_manager"`
	Etcd         *Etcd         `mapstructure:"etcd"`
}

type Eviction struct {
	CleanUpInterval time.Duration `mapstructure:"clean_up_interval"`
	TTL             time.Duration `mapstructure:"ttl"`
	NumSegments     int           `mapstructure:"num_segments"`
}

type GroupManager struct {
	Strategy     string `mapstructure:"strategy"`
	MaxCacheSize int64  `mapstructure:"max_cache_size"`
}

type SingleFlight struct {
	TTL time.Duration `mapstructure:"ttl"`
}

type Etcd struct {
	Address []string `mapstructure:"address"`
	TTL     int      `mapstructure:"ttl"`
}

var once sync.Once

func init() {
	once.Do(func() {
		if err := newViperConfig(); err != nil {
			panic(err)
		}
	})
}

func newViperConfig() (err error) {
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
	//fmt.Printf("caller from: %s, here: %s, relPath: %s\n", callerPwd, here, relPath)
	return
}

func initClientV3Config() {
	DefaultEtcdConfig = clientv3.Config{
		Endpoints:   Conf.Etcd.Address,
		DialTimeout: 5 * time.Second,
	}
}
