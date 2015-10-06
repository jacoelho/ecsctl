package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"k8s.io/kubernetes/pkg/util/wait"
)

var (
	notFound      = errors.New("service not found")
	multipleFound = errors.New("multiple container definitions found")
)

type configInteractor struct {
	cluster string
	region  string
}

type ecsInteractor struct {
	ecs     *ecs.ECS
	cluster string
}

type ecsService struct {
	pendingCount   int64
	runningCount   int64
	serviceName    string
	status         string
	taskDefinition string
}

type ecsUpdateConfig struct {
	prevService  string
	nextService  string
	image        string
	count        int
	timeout      time.Duration
	updatePeriod time.Duration
}

func newInteractor(c configInteractor) *ecsInteractor {
	svc := ecs.New(aws.NewConfig().WithRegion(c.region))

	return &ecsInteractor{
		ecs:     svc,
		cluster: c.cluster,
	}
}

func (e *ecsInteractor) checkService(s string) (*ecsService, error) {
	resp, err := e.ecs.DescribeServices(
		&ecs.DescribeServicesInput{
			Services: []*string{
				aws.String(s),
			},
			Cluster: aws.String(e.cluster),
		},
	)

	if err != nil {
		return nil, err
	}

	if len(resp.Services) != 1 {
		return nil, notFound
	}

	return &ecsService{
		pendingCount:   *resp.Services[0].PendingCount,
		runningCount:   *resp.Services[0].RunningCount,
		serviceName:    *resp.Services[0].ServiceName,
		status:         *resp.Services[0].Status,
		taskDefinition: *resp.Services[0].TaskDefinition,
	}, nil
}

func (e *ecsInteractor) createTask(taskName, imageName string) (*string, error) {
	task, err := e.ecs.DescribeTaskDefinition(
		&ecs.DescribeTaskDefinitionInput{
			TaskDefinition: aws.String(taskName),
		},
	)

	if err != nil {
		return nil, err
	}

	if len(task.TaskDefinition.ContainerDefinitions) > 1 {
		return nil, multipleFound
	}

	// update image name
	if len(imageName) > 0 {
		task.TaskDefinition.ContainerDefinitions[0].Image = aws.String(imageName)
	}

	resp, err := e.ecs.RegisterTaskDefinition(
		&ecs.RegisterTaskDefinitionInput{
			ContainerDefinitions: task.TaskDefinition.ContainerDefinitions,
			Family:               task.TaskDefinition.Family,
			Volumes:              task.TaskDefinition.Volumes,
		},
	)

	if err != nil {
		return nil, err
	}
	return resp.TaskDefinition.TaskDefinitionArn, nil
}

func (e *ecsInteractor) createService(name string, taskDefinition *string) error {
	_, err := e.ecs.CreateService(
		&ecs.CreateServiceInput{
			DesiredCount:   aws.Int64(1),
			ServiceName:    aws.String(name),
			TaskDefinition: taskDefinition,
			Cluster:        aws.String(e.cluster),
		},
	)

	if err != nil {
		return err
	}
	return nil
}

func (e *ecsInteractor) deleteService(name string) error {
	_, err := e.ecs.DeleteService(
		&ecs.DeleteServiceInput{
			Service: aws.String(name),
			Cluster: aws.String(e.cluster),
		},
	)

	if err != nil {
		return err
	}
	return nil
}

func (e *ecsInteractor) updateService(name string, count int64) error {
	_, err := e.ecs.UpdateService(
		&ecs.UpdateServiceInput{
			Service:      aws.String(name),
			Cluster:      aws.String(e.cluster),
			DesiredCount: aws.Int64(count),
		},
	)

	if err != nil {
		return err
	}

	return nil
}

func (e *ecsInteractor) waitForUpdate(service string) (bool, error) {
	check, err := e.checkService(service)
	if err != nil {
		return false, err
	}

	if check.pendingCount > 0 {
		return false, nil
	}

	return true, nil
}

func (interactor *ecsInteractor) rollingUpdate(config ecsUpdateConfig) {
	prevServiceName := config.prevService
	nextServiceName := config.nextService

	service, err := interactor.checkService(prevServiceName)
	if err != nil {
		fmt.Printf("unknown service %s\n", prevServiceName)
		return
	}

	if service.runningCount == 0 || service.status == "INACTIVE" {
		fmt.Println("service not running")
		return
	}

	newTask, err := interactor.createTask(service.taskDefinition, config.image)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	newService, err := interactor.checkService(nextServiceName)
	if err == notFound {
		fmt.Println("creating a new service")
		interactor.createService(nextServiceName, newTask)
	} else if newService.status == "INACTIVE" {
		fmt.Println("service inactive")
		fmt.Println("creating service with timestamp")
		nextServiceName = fmt.Sprintf("%s-%s", prevServiceName, time.Now().Format("20060102150405"))
		interactor.createService(nextServiceName, newTask)
	}

	desiredCount := service.runningCount

	if config.count > 0 {
		desiredCount = int64(config.count)
	}

	for {
		prevService, err := interactor.checkService(prevServiceName)
		nextService, err := interactor.checkService(nextServiceName)

		if err != nil {
			return
		}

		prevCount := prevService.runningCount
		nextCount := nextService.runningCount

		if nextCount >= desiredCount {
			if prevCount != 0 {
				interactor.updateService(prevServiceName, 0)
				wait.Poll(5*time.Second, config.timeout, func() (bool, error) { return interactor.waitForUpdate(prevServiceName) })
			}

			interactor.deleteService(prevServiceName)
			break
		}

		interactor.updateService(nextServiceName, nextCount+1)
		wait.Poll(10*time.Second, config.timeout, func() (bool, error) { return interactor.waitForUpdate(nextServiceName) })

		interactor.updateService(prevServiceName, prevCount-1)
		wait.Poll(10*time.Second, config.timeout, func() (bool, error) { return interactor.waitForUpdate(prevServiceName) })

		fmt.Printf("container started: waiting %s\n", config.updatePeriod)
		time.Sleep(config.updatePeriod)
	}
}
