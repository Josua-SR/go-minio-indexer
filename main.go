package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/maltegrosse/go-minio-list/controllers"
	. "github.com/maltegrosse/go-minio-list/log"
	"github.com/maltegrosse/go-minio-list/models"
	"github.com/maltegrosse/go-minio-list/routers"
	"github.com/spf13/viper"
)

type Options struct {
	ConfigDir string `short:"c" long:"config-directory" default:"config" description:"location of configuration files"`
}

func main() {
	var err error
	var options Options

	// parse cli options
	_, err = flags.Parse(&options)
	if err != nil {
		os.Exit(1)
	}

	// load config
	viper.AddConfigPath(options.ConfigDir)
	err = viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	models.Endpoint = viper.GetString("minio_server")
	models.PublicEndpoint = viper.GetString("public_minio_server")
	models.AccessKeyID = viper.GetString("minio_access_key")
	models.SecretAccessKey = viper.GetString("minio_secret_key")
	models.UseSSL = viper.GetBool("minio_secure")
	models.PublicUseSSL = viper.GetBool("public_minio_secure")
	models.BucketName = viper.GetString("minio_bucket_name")
	if models.UseSSL {
		models.DirectUrl = "https://" + models.Endpoint + "/" + models.BucketName
	} else {
		models.DirectUrl = "http://" + models.Endpoint + "/" + models.BucketName
	}
	if models.PublicUseSSL {
		models.PublicUrl = "https://" + models.PublicEndpoint + "/" + models.BucketName
	} else {
		models.PublicUrl = "http://" + models.PublicEndpoint + "/" + models.BucketName
	}
	models.MetaFilename = viper.GetString("meta_filename")
	models.ReverseProxy = viper.GetBool("reverse_proxy")
	err = checkStorage()
	if err != nil {
		panic(fmt.Errorf("Minio not available: %s \n", err))
	}
	apiConnection := viper.GetString("service_host") + ":" + viper.GetString("service_port")

	// start directory indexing in background
	Log.Info("Starting index updater")
	go updateIndex()

	Log.Info("Starting service on: " + apiConnection)
	err = http.ListenAndServe(apiConnection, routers.Routes())
	if err != nil {
		panic(err)
	}

}

func updateIndex() {
	var tick <-chan time.Time

	// get index once
	Log.Info("Building initial Index")
	controllers.Index = models.IndexBucket()

	// then once per minute
	tick = time.Tick(time.Minute)
	for {
		select {
		case <-tick:
			// update index
			// TODO: only if changed
			Log.Info("Rebuilding Index")
			controllers.Index = models.IndexBucket()
		}
	}
}

func checkStorage() error {
	minioClient, err := models.CreateMinioClient()
	if err != nil {
		Log.Error(err.Error())
		return err
	}
	_, err = minioClient.GetBucketPolicy(models.BucketName)
	if err != nil {
		Log.Warn("Failed to retrieve bucket policy: " + err.Error())
		return nil
	}
	return nil
}
