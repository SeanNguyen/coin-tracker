package main

import (
	"github.com/garyburd/redigo/redis"
	"github.com/gocraft/work"
	"github.com/seannguyen/coin-tracker/internal/jobs"
	"github.com/seannguyen/coin-tracker/internal/utilities"
	"github.com/spf13/viper"
	"github.com/volatiletech/sqlboiler/boil"
	"log"
	"os"
	"os/signal"
)

type Context struct{}

func main() {
	initConfigs()
	initJobs()
}

func initConfigs() {
	log.Println("Initializing configs")
	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.AddConfigPath("./configs")
	err := viper.ReadInConfig()
	if err != nil {
		log.Panicln(err)
	}
	if utilities.IsDevelopment() {
		boil.DebugMode = true
	}
}

func initJobs() {
	log.Println("Initializing jobs")
	redisPool := createRedisPool()
	pool := work.NewWorkerPool(Context{}, 2, "coin-tracker", redisPool)

	pool.Middleware(logJobStartEvent)

	pool.JobWithOptions("snapshot_balances", work.JobOptions{MaxConcurrency: 1}, jobs.SnapshotBalances)
	pool.PeriodicallyEnqueue("0 */15 * * * *", "snapshot_balances")
	pool.Start()

	// Wait for a signal to quit:
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	<-signalChan

	// Stop the pool
	pool.Stop()
	redisPool.Close()
}

func logJobStartEvent(job *work.Job, next work.NextMiddlewareFunc) error {
	log.Printf("Starting job: %s", job.Name)
	return next()
}

func createRedisPool() *redis.Pool {
	log.Println("Creating redis connection pool")
	return &redis.Pool{
		MaxActive: 5,
		MaxIdle:   5,
		Wait:      true,
		Dial: func() (redis.Conn, error) {
			connection, err := redis.Dial("tcp", viper.GetString("REDIS_ADDRESS"))
			if err != nil {
				connection.Close()
				return nil, err
			}
			password := viper.GetString("REDIS_PASSWORD")
			if len(password) > 0 {
				if _, err := connection.Do("AUTH", password); err != nil {
					connection.Close()
					return nil, err
				}
			}
			if _, err := connection.Do("SELECT", viper.GetString("REDIS_DB")); err != nil {
				connection.Close()
				return nil, err
			}
			return connection, nil
		},
	}
}
