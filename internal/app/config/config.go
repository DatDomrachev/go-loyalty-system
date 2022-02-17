package config

import (
	"flag"
	"github.com/caarlos0/env/v6"
	"log"
)

type Config struct {
	Address     string `env:"RUN_ADDRESS" envDefault:"localhost:8080"`
	DBURL       string `env:"DATABASE_URI" envDefault:""`
	AccrualURL  string `env:"ACCRUAL_SYSTEM_ADDRESS" envDefault:""`
}

func New() (*Config, error) {
	cfg := &Config{}
	err := env.Parse(cfg)
	if err != nil {
		log.Printf("env prsing failed:+%v\n", err)
		return nil, err
	}

	return cfg, nil
}

func (c *Config) InitFlags() {
	flag.StringVar(&c.Address, "a", c.Address, "host to listen on")
	flag.StringVar(&c.DBURL, "d", c.DBURL, "data base url")
	flag.StringVar(&c.AccrualURL, "r", c.AccrualURL, "data base url")
	flag.Parse()
}
