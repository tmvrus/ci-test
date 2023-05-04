package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/caarlos0/env"
)

type Config struct {
	AgentConfig  AgentConfig
	ServerConfig ServerConfig
}

type AgentConfig struct {
	Address        string        `env:"ADDRESS" envDefault:"127.0.0.1:8080"`
	ReportInterval time.Duration `env:"REPORT_INTERVAL" envDefault:"10s"`
	PollInterval   time.Duration `env:"POLL_INTERVAL" envDefault:"2s"`
	Key            string        `enc:"KEY" envDefault:""`
}

type ServerConfig struct {
	Address       string        `env:"ADDRESS" envDefault:"127.0.0.1:8080"`
	StoreInterval time.Duration `env:"STORE_INTERVAL" envDefault:"300s"`
	StoreFile     string        `env:"STORE_FILE" envDefault:"/tmp/devops-metrics-db.json"`
	Restore       bool          `env:"RESTORE" envDefault:"true"`
	Key           string        `enc:"KEY" envDefault:""`
	DBDSN         string        `env:"DATABASE_DSN"`
}

func NewConfig(t string) (*Config, error) {
	var cfg Config
	switch t {
	case "agent":
		if err := env.Parse(&cfg.AgentConfig); err != nil {
			return nil, fmt.Errorf("unable load env vars. will use default values. error: %+v", err)
		}

		var (
			address, key string
			report, poll time.Duration
		)

		flag.StringVar(&address, "a", "127.0.0.1:8080", "Please provide server Address in form '127.0.0.1:8080'")
		flag.DurationVar(&report, "r", 10*time.Second, "Please provide Report Interval in form '10s'")
		flag.DurationVar(&poll, "p", 2*time.Second, "Please provide Poll interval in form '2s'")
		flag.StringVar(&key, "k", "", "Please provide Key for sign")

		flag.Parse()
		if !isEnvExist("ADDRESS") && address != "" {
			cfg.AgentConfig.Address = address
		}
		if !isEnvExist("KEY") && key != "" {
			cfg.AgentConfig.Key = key
		}
		if !isEnvExist("REPORT_INTERVAL") && report != 0 {
			cfg.AgentConfig.ReportInterval = report
		}
		if !isEnvExist("POLL_INTERVAL") && poll != 0 {
			cfg.AgentConfig.PollInterval = poll
		}
		log.Printf("Starting agent with following configs: %+v", cfg.AgentConfig)

	case "server":
		if err := env.Parse(&cfg.ServerConfig); err != nil {
			return nil, fmt.Errorf("unable load env vars. will use default values. error: %+v", err)
		}
		var (
			address, file, key, db string
			interval               time.Duration
			restore                bool
		)
		flag.StringVar(&address, "a", "127.0.0.1:8080", "Please provide server Address in form '127.0.0.1:8080'")
		flag.DurationVar(&interval, "i", 300*time.Second, "Please provide store interval in form '300s'")
		flag.BoolVar(&restore, "r", true, "Please provide server Address in form 'true/false'")
		flag.StringVar(&file, "f", "/tmp/devops-metrics-db.json", "Please provide server Address in form '/path/to/file.json'")
		flag.StringVar(&key, "k", "", "Please provide Key for sign")
		flag.StringVar(&db, "d", "", "Please provide DB DSN")

		flag.Parse()

		if !isEnvExist("ADDRESS") && address != "" {
			cfg.ServerConfig.Address = address
		}
		if !isEnvExist("KEY") && key != "" {
			cfg.ServerConfig.Key = key
		}
		if !isEnvExist("DATABASE_DSN") && db != "" {
			cfg.ServerConfig.DBDSN = db
		}
		if !isEnvExist("STORE_INTERVAL") && interval != 0 {
			cfg.ServerConfig.StoreInterval = interval
		}
		if !isEnvExist("RESTORE") {
			cfg.ServerConfig.Restore = restore
		}
		if !isEnvExist("STORE_FILE") && file != "" {
			cfg.ServerConfig.StoreFile = file
		}
		log.Printf("Starting server with following configs: %+v", cfg.ServerConfig)

	}

	return &cfg, nil
}

func isEnvExist(key string) bool {
	if _, ok := os.LookupEnv(key); ok {
		return true
	}
	return false
}
