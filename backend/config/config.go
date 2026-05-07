package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Security SecurityConfig `mapstructure:"security"`
	Polling  PollingConfig  `mapstructure:"polling"`
	Dell     DellConfig     `mapstructure:"dell_catalog"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type SecurityConfig struct {
	MasterKeyPath string `mapstructure:"master_key_path"`
	MasterKeyEnv  string `mapstructure:"master_key_env"`
}

type PollingConfig struct {
	SystemIntervalSeconds    int `mapstructure:"system_interval_seconds"`
	ThermalIntervalSeconds   int `mapstructure:"thermal_interval_seconds"`
	PowerIntervalSeconds     int `mapstructure:"power_interval_seconds"`
	BiosIntervalHours        int `mapstructure:"bios_interval_hours"`
	FirmwareIntervalHours    int `mapstructure:"firmware_interval_hours"`
	StorageIntervalMinutes   int `mapstructure:"storage_interval_minutes"`
	JobQueueIntervalSeconds  int `mapstructure:"job_queue_interval_seconds"`
	CatalogUpdateHour        int `mapstructure:"catalog_update_hour"`
	SSEReconnectMaxRetries   int `mapstructure:"sse_reconnect_max_retries"`
}

type DellConfig struct {
	CatalogURL  string `mapstructure:"url"`
	CachePath   string `mapstructure:"cache_path"`
}

func Load() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/data")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("config file not found, using defaults: %v", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}
	return &cfg
}

func setDefaults() {
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("database.path", "/data/database.sqlite")
	viper.SetDefault("security.master_key_path", "/data/master.key")
	viper.SetDefault("polling.system_interval_seconds", 30)
	viper.SetDefault("polling.thermal_interval_seconds", 60)
	viper.SetDefault("polling.power_interval_seconds", 30)
	viper.SetDefault("polling.bios_interval_hours", 1)
	viper.SetDefault("polling.firmware_interval_hours", 6)
	viper.SetDefault("polling.storage_interval_minutes", 10)
	viper.SetDefault("polling.job_queue_interval_seconds", 15)
	viper.SetDefault("polling.catalog_update_hour", 3)
	viper.SetDefault("polling.sse_reconnect_max_retries", 5)
	viper.SetDefault("dell_catalog.url", "https://downloads.dell.com/catalog/Catalog.xml.gz")
	viper.SetDefault("dell_catalog.cache_path", "/data/catalog.xml.gz")
}
