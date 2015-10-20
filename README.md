# ecsctl

ecsctl - rolling deployments on AWS ECS service.

## Demo

![demo](https://github.com/jacoelho/ecsctl/blob/master/images/ecsctl.gif)

## Usage

```bash
ecsctl [--cluster <cluster name>] [--region <aws region>] rolling-update <service> [--timeout 60] [--update-period 45] [--instance-count 3] [--image <new image>]
```

For example:

```bash
ecsctl --cluster frontend rolling-update nginx --image nginx:2433e41
```

This will create a new service called nginx-blue with the task updated to the image: nginx:2433e41

Running again:

```bash
ecsctl --cluster frontend rolling-update nginx --image nginx:latest
```
This will create a new service called nginx-green with the task updated to the image: nginx:latest
