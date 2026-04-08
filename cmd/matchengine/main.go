package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/consumer"
	"github.com/hzx/matchengine/internal/engine"
	"github.com/hzx/matchengine/internal/producer"
	"github.com/hzx/matchengine/internal/server"
	"github.com/hzx/matchengine/internal/storage"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	configPath = flag.String("config", "./configs/config.yaml", "config file path")
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger, err := initLogger(cfg)
	if err != nil {
		fmt.Printf("failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 初始化MySQL存储
	mysqlStorage, err := storage.NewMySQLStorage(&cfg.MySQL, logger)
	if err != nil {
		logger.Fatal("failed to init mysql storage", zap.Error(err))
	}
	defer mysqlStorage.Close()

	// 初始化 Kafka Producer (同步)
	var kafkaProducer *producer.KafkaProducer
	if cfg.Kafka.EnableProducer {
		kafkaProducer, err = producer.NewKafkaProducer(&cfg.Kafka, logger)
		if err != nil {
			logger.Fatal("failed to init kafka producer", zap.Error(err))
		}
		defer kafkaProducer.Close()
		logger.Info("kafka producer (sync) initialized")
	}

	// 初始化 Kafka Producer (异步) - 可选
	var asyncKafkaProducer *producer.AsyncProducer
	if cfg.Kafka.EnableProducer && cfg.Kafka.UseAsyncProducer {
		asyncKafkaProducer, err = producer.NewAsyncProducer(&cfg.Kafka, logger)
		if err != nil {
			logger.Warn("failed to init async kafka producer, using sync mode", zap.Error(err))
			asyncKafkaProducer = nil
		} else {
			defer asyncKafkaProducer.Close()
			logger.Info("kafka producer (async) initialized")
		}
	}

	// 创建事件处理器链
	storageHandler := engine.NewStorageHandler(mysqlStorage, logger)
	kafkaHandler := engine.NewKafkaHandler(kafkaProducer, asyncKafkaProducer, cfg.Kafka.UseAsyncProducer, logger)
	eventChain := engine.NewEventHandlerChain(storageHandler, kafkaHandler)

	// 创建撮合引擎
	matchingEngine := engine.NewEngine(cfg, logger, eventChain)

	// 启动撮合引擎
	if err := matchingEngine.Start(); err != nil {
		logger.Fatal("failed to start matching engine", zap.Error(err))
	}
	defer matchingEngine.Stop()

	// 启动 Kafka Consumer (如果启用)
	var kafkaConsumer *consumer.KafkaConsumer
	if cfg.Kafka.EnableConsumer {
		kafkaConsumer, err = consumer.NewKafkaConsumer(&cfg.Kafka, logger, matchingEngine)
		if err != nil {
			logger.Fatal("failed to init kafka consumer", zap.Error(err))
		}
		defer kafkaConsumer.Close()
		logger.Info("kafka consumer started")
	}

	// 启动gRPC服务
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.GRPCPort))
	if err != nil {
		logger.Fatal("failed to listen grpc", zap.Error(err))
	}
	grpcServer := grpc.NewServer()
	server.RegisterGRPCServices(grpcServer, matchingEngine, mysqlStorage, logger)

	go func() {
		logger.Info("grpc server started", zap.Int("port", cfg.Server.GRPCPort))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("grpc server error", zap.Error(err))
		}
	}()

	// 启动HTTP服务
	e := server.NewEchoServer(matchingEngine, mysqlStorage, logger)
	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)

	go func() {
		// 注册Prometheus指标
		e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
		logger.Info("http server started", zap.Int("port", cfg.Server.HTTPPort))
		if err := e.Start(httpAddr); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", zap.Error(err))
		}
	}()

	// 等待信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// 优雅关闭
	grpcServer.GracefulStop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.Shutdown(ctx)

	logger.Info("server stopped")
}

func initLogger(cfg *config.Config) (*zap.Logger, error) {
	var zapConfig zap.Config
	if cfg.Log.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	switch cfg.Log.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return zapConfig.Build()
}
