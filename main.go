package main

import (
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/ghibranalj/signifer/db/sqlc"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/viper"
	_ "modernc.org/sqlite"
)

//go:embed db/migrations
var migrationsFS embed.FS

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

	if err := runMigrations(cfg.DB.Path); err != nil {
		log.Fatal(err)
	}

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

func runMigrations(dbPath string) error {
	tempDir, err := os.MkdirTemp("", "signifer-migrations-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	migrations, err := fs.Sub(migrationsFS, "db/migrations")
	if err != nil {
		return err
	}

	fs.WalkDir(migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		content, err := fs.ReadFile(migrations, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return err
		}
		return nil
	})

	m, err := migrate.New(
		"file://"+tempDir,
		"sqlite3://"+dbPath,
	)
	if err != nil {
		return err
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}

	log.Println("Database migrations completed")
	return nil
}
