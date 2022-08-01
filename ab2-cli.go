package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	p "path"
	"path/filepath"
	"strings"
	"time"
)

var filetype string
var protocol string
var path string
var apiGWUrl string
var ingestBucket string
var ipfsGW string
var httpsProxy string

func triggerM2c(cCtx *cli.Context) error {
	cfg, err := config.LoadDefaultConfig(context.TODO())

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	credentials, err := cfg.Credentials.Retrieve(context.TODO())

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	values := map[string]string{"bucket": ingestBucket, "key": path}
	jsonValue, _ := json.Marshal(values)
	fmt.Printf("Json string %v\n", values)

	sha_sum := sha256.Sum256(jsonValue)
	hash := fmt.Sprintf("%x", sha_sum)
	fmt.Printf("Hash %s\n", hash)

	// The signer requires a payload hash. This hash is for an empty payload.
	//hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req, _ := http.NewRequest(http.MethodPost, apiGWUrl, bytes.NewBuffer(jsonValue))
	req.Header.Add("Content-Type", "application/json")

	signer := v4.NewSigner()
	err = signer.SignHTTP(context.TODO(), credentials, req, hash, "execute-api", cfg.Region, time.Now())

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	dump, _ := httputil.DumpRequestOut(req, true)
	fmt.Println(string(dump))

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Printf("failed to call remote service: (%v)\n", err)
		return cli.Exit(err, 1)
	}

	defer res.Body.Close()
	fmt.Printf("%v", res)

	if res.StatusCode != 200 {
		return cli.Exit(err, 1)
	} else {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			log.Fatal(err)
		}
		bodyString := string(bodyBytes)
		fmt.Println(bodyString)
	}

	return nil
}

func upload2s3(cCtx *cli.Context) error {
	switch protocol {
	case "local":
		return uploadFromLocal(path, filepath.Base(path))
	case "ipfs":
		localPath, err := downloadFromIpfs(path)
		if err != nil {
			return err
		}
		return uploadFromLocal(localPath, path+"."+filetype)
	case "http":
		filename, localPath, err := downloadFromHttp(path)
		if err != nil {
			return err
		}
		return uploadFromLocal(localPath, filename)
	default:
		return fmt.Errorf("Not implemented")
	}

	return nil
}

func downloadFromHttp(path string) (string, string, error) {
	// Get the data
	if strings.HasPrefix(httpsProxy, "http") {
		os.Setenv("HTTPS_PROXY", httpsProxy)
	}
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	filename := p.Base(req.URL.Path) + "." + filetype
	localpath, err := copyHttpBodyToTempFile(req, filename)
	os.Unsetenv("HTTPS_PROXY")
	return filename, localpath, err
}

func downloadFromIpfs(path string) (string, error) {
	req, _ := http.NewRequest(http.MethodPost, ipfsGW, nil)
	q := req.URL.Query()
	q.Add("arg", path)
	req.URL.RawQuery = q.Encode()
	dump, _ := httputil.DumpRequestOut(req, false)
	fmt.Println(string(dump))
	filename := path + "." + filetype
	return copyHttpBodyToTempFile(req, filename)
}

func copyHttpBodyToTempFile(req *http.Request, filename string) (string, error) {
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return "", err
	}
	fmt.Printf("Response: %v\n", resp)

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error fetching from IPFS with status code %d", resp.StatusCode)
	}

	fmt.Printf("Filename: %s", filename)
	out, err := ioutil.TempFile("", filename)
	filename = out.Name()

	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return filename, nil
}

func uploadFromLocal(path string, filename string) error {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return cli.Exit(err, 1)
	}
	client := s3.NewFromConfig(cfg)
	f, err := os.Open(path)
	if err != nil {
		return cli.Exit(err, 1)
	}

	input := &s3.PutObjectInput{
		Body:   f,
		Bucket: aws.String(ingestBucket),
		Key:    aws.String(filename),
	}
	result, err := client.PutObject(context.TODO(), input)
	fmt.Printf("Upload result: (%v)\n", result)
	if err != nil {
		return cli.Exit(err, 1)
	}

	fmt.Println("Succeeded")
	return nil
}

func main() {
	homedir, _ := os.UserHomeDir()
	viper.SetConfigFile(homedir + "/go/bin/config.yaml")
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	apiGWUrl = fmt.Sprintf("%v", viper.Get("m2c-url"))
	ingestBucket = fmt.Sprintf("%v", viper.Get("ingest-bucket"))
	ipfsGW = fmt.Sprintf("%v", viper.Get("ipfs-gateway"))
	httpsProxy = fmt.Sprintf("%v", viper.Get("https-proxy"))

	uploadCommand := cli.Command{
		Name:    "upload",
		Aliases: []string{"u"},
		Usage:   "upload to s3",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "filetype",
				Aliases:     []string{"f"},
				Value:       "csv",
				Usage:       "file type, possible values `csv`|`png`|`jpg`",
				Destination: &filetype,
			},
			&cli.StringFlag{
				Name:        "protocol",
				Aliases:     []string{"p"},
				Value:       "local",
				Usage:       "file type, possible values `local`|`ipfs`｜`http`",
				Destination: &protocol,
			},
			&cli.StringFlag{
				Name:        "path",
				Aliases:     []string{"u"},
				Usage:       "file path",
				Destination: &path,
				Required:    true,
			},
		},
		Action: upload2s3,
	}

	processCommand := cli.Command{
		Name:    "process",
		Aliases: []string{"p"},
		Usage:   "trigger m2c process",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "path",
				Aliases:     []string{"u"},
				Usage:       "file path",
				Destination: &path,
				Required:    true,
			},
		},
		Action: triggerM2c,
	}

	app := &cli.App{
		Name:     "ab2 command line",
		Commands: []*cli.Command{&uploadCommand, &processCommand},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
