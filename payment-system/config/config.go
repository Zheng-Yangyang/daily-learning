package config

type Config struct {
	MySQL MySQLConfig
	Redis RedisConfig
	Kafka KafkaConfig
	HTTP  HTTPConfig
}

type MySQLConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr string
}

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

type HTTPConfig struct {
	Port int
}

func Load() *Config {
	return &Config{
		MySQL: MySQLConfig{
			DSN: "root:zyy123456@tcp(127.0.0.1:3306)/payment_system?parseTime=true&charset=utf8mb4",
		},
		Redis: RedisConfig{
			Addr: "127.0.0.1:6379",
		},
		Kafka: KafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
			Topic:   "payment-events",
		},
		HTTP: HTTPConfig{
			Port: 8080,
		},
	}
}
