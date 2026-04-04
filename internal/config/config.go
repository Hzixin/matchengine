package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Kafka   KafkaConfig   `mapstructure:"kafka"`
	Redis   RedisConfig   `mapstructure:"redis"`
	MySQL   MySQLConfig   `mapstructure:"mysql"`
	Raft    RaftConfig    `mapstructure:"raft"`
	Log     LogConfig     `mapstructure:"log"`
	Markets []MarketConfig `mapstructure:"markets"`
}

type ServerConfig struct {
	GRPCPort int `mapstructure:"grpc_port"`
	HTTPPort int `mapstructure:"http_port"`
}

type KafkaConfig struct {
	Brokers       []string `mapstructure:"brokers"`
	OrderTopic    string   `mapstructure:"order_topic"`
	TradeTopic    string   `mapstructure:"trade_topic"`
	ConsumerGroup string   `mapstructure:"consumer_group"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

type RaftConfig struct {
	NodeID  uint64   `mapstructure:"node_id"`
	DataDir string   `mapstructure:"data_dir"`
	Peers   []string `mapstructure:"peers"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type MarketConfig struct {
	Symbol        string `mapstructure:"symbol"`
	BaseAsset     string `mapstructure:"base_asset"`
	QuoteAsset    string `mapstructure:"quote_asset"`
	BasePrecision int    `mapstructure:"base_precision"`
	QuotePrecision int   `mapstructure:"quote_precision"`
	MinAmount     string `mapstructure:"min_amount"`
	MinTotal      string `mapstructure:"min_total"`
	PriceTick     string `mapstructure:"price_tick"`
	AmountTick    string `mapstructure:"amount_tick"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
