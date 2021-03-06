package main

import (
	"fmt"
	"os"
	"time"

	"github.com/codegangsta/cli"
)

var (
	Version string
)

func ecsCli(c *cli.Context) {
	service := c.Args().First()
	cluster := c.GlobalString("cluster")
	region := c.GlobalString("region")
	image := c.String("image")
	timeout := c.Int("timeout")
	updatePeriod := c.Int("update-period")
	instanceCount := c.Int("instance-count")

	switch {
	case len(service) == 0:
		fmt.Println("invalid service name")
		return
	case len(region) == 0:
		fmt.Println("invalid aws region")
		return
	}

	interactor := newInteractor(configInteractor{cluster: cluster, region: region})
	interactor.rollingUpdate(ecsUpdateConfig{
		prevService:  service,
		image:        image,
		timeout:      time.Duration(timeout) * time.Second,
		updatePeriod: time.Duration(updatePeriod) * time.Second,
		count:        instanceCount,
	},
	)
}

func main() {
	app := cli.NewApp()
	app.Name = "ecsctl"
	app.Version = Version
	app.Usage = "rolling-update <service> <next service> [--timeout 60] [--update-period 45] [--instance-count 3]"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "cluster",
			Value: "default",
			Usage: "ecs cluster",
		},
		cli.StringFlag{
			Name:   "region",
			Usage:  "AWS region",
			EnvVar: "AWS_DEFAULT_REGION",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:   "rolling-update",
			Usage:  "service to update",
			Action: ecsCli,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "image",
				},
				cli.IntFlag{
					Name:  "timeout",
					Value: 60,
				},
				cli.IntFlag{
					Name:  "update-period",
					Value: 30,
				},
				cli.IntFlag{
					Name: "instance-count",
				},
			},
		},
	}

	app.Run(os.Args)
}
