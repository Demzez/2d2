package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	TCPServer
	DataBase
	JWTSecret     string
	OTPSecret     string
	PaymentSecret string
	DevMode       bool
	AllowedOrigin string
}

type TCPServer struct {
	Host string
	Port int
}

type DataBase struct {
	Path string
}

func MustLoad() *Config {
	port, err := strconv.Atoi(getEnv("PORT", "5555"))
	if err != nil || port < 1 || port > 65535 {
		log.Fatal("invalid PORT")
	}

	cfg := &Config{
		TCPServer:     TCPServer{Host: getEnv("HOST", "127.0.0.1"), Port: port},
		DataBase:      DataBase{Path: os.Getenv("DATABASE_URL")},
		JWTSecret:     os.Getenv("JWT_SECRET"),
		OTPSecret:     os.Getenv("OTP_SECRET"),
		PaymentSecret: os.Getenv("PAYMENT_SECRET"),
		DevMode:       os.Getenv("DEV_MODE") == "true",
		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "http://localhost:5173"),
	}

	if cfg.Path == "" || len(cfg.JWTSecret) < 32 || len(cfg.OTPSecret) < 32 || len(cfg.PaymentSecret) < 32 {
		log.Fatal("DATABASE_URL and JWT_SECRET, OTP_SECRET, PAYMENT_SECRET (at least 32 characters) are required")
	}

	return cfg
}

func getEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
