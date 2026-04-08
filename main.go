package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	"github.com/ghibranalj/signifer/db/sqlc"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/viper"
	_ "modernc.org/sqlite"
)

type Config struct {
	Port    int `mapstructure:"port"`

	Discord struct {
		WebhookURL string `mapstructure:"webhook_url"`
	} `mapstructure:"discord"`

	Auth struct {
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`
	} `mapstructure:"auth"`

	Ping struct {
		IntervalSeconds int `mapstructure:"interval_seconds"`
		FailedThreshold int `mapstructure:"failed_threshold"`
	}

	DB struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"db"`
}

var cfg Config

func init() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Set defaults
	viper.SetDefault("port", 9090)
	viper.SetDefault("auth.user", "admin")
	viper.SetDefault("auth.password", "admin")
	viper.SetDefault("ping.interval_seconds", 30)
	viper.SetDefault("ping.failed_threshold", 3)
	viper.SetDefault("db.path", "./data/signifer.db")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Config file not found, using defaults")
		} else {
			log.Fatalf("Error reading config file: %v", err)
		}
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Error unmarshaling config: %v", err)
	}
}

func main() {

	folder := filepath.Dir(cfg.DB.Path)
	err := os.MkdirAll(folder, 0755)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite", cfg.DB.Path)
	if err != nil {
		log.Fatal(err)
	}

	// Run database migrations
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		log.Fatal(err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"sqlite",
		driver,
	)
	if err != nil {
		log.Fatal(err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
	}
	log.Println("Database migrations completed")

	queries := sqlc.New(db)

	rest := &Rest{
		repo:     queries,
		User:     cfg.Auth.User,
		Password: cfg.Auth.Password,
	}

	// Discord webhook is required
	if cfg.Discord.WebhookURL == "" {
		log.Fatal("Discord webhook URL is required. Please set discord.webhook_url in config.yaml")
	}
	discordClient := NewDiscord(cfg.Discord.WebhookURL)
	log.Println("Discord webhook configured")

	// Start background pinger with Discord client
	pinger := NewPinger(queries, cfg.Ping.IntervalSeconds, cfg.Ping.FailedThreshold, discordClient)
	pinger.Start()
	defer pinger.Stop()

	log.Printf("Server starting on port %d", cfg.Port)
	if err := rest.Start(cfg.Port); err != nil {
		log.Fatal(err)
	}
}
