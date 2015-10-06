# ecsctl

ecsctl - rolling deployments on AWS ECS service.

## Usage

```bash
ecsctl [--cluster <cluster name>] [--region <aws region>] rolling-update <service> <next service> [--timeout 60] [--update-period 45] [--instance-count 3] [--image <new image>]
```


For example:

```bash
ecsctl --cluster frontend rolling-update nginx-v10 nginx-v11 --image nginx:2433e41
```
