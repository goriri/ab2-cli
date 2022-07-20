package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	var filetype string
	var protocol string
	var path string

	processAction := func(cCtx *cli.Context) error {
		switch protocol {
		case "local":
			content, err := ioutil.ReadFile(path)
			if err != nil {
				log.Fatal(err)
				return err
			}
			fmt.Println(string(content))
		default:
			return fmt.Errorf("Not implemented")
		}
		return nil
	}

	processCommand := cli.Command{
		Name:    "process",
		Aliases: []string{"p"},
		Usage:   "process a file",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "filetype",
				Aliases:     []string{"f"},
				Value:       "image",
				Usage:       "file type, possible values `image`|`text`",
				Destination: &filetype,
			},
			&cli.StringFlag{
				Name:        "protocol",
				Aliases:     []string{"p"},
				Value:       "local",
				Usage:       "file type, possible values `local`|`ipfs`",
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
		Action: processAction,
	}

	app := &cli.App{
		Name:     "ab2 command line",
		Commands: []*cli.Command{&processCommand},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
