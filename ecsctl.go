package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"k8s.io/kubernetes/pkg/util/wait"
)

const (
	BLUE  = "blue"
	GREEN = "green"
)

var (
	notFound      = errors.New("service not found")
	multipleFound = errors.New("multiple container definitions found")
	colourAssign  = errors.New("unable to determine next colour")
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

	// update image name?
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

func (e *ecsInteractor) updateService(service, task string, count int64) error {
	_, err := e.ecs.UpdateService(
		&ecs.UpdateServiceInput{
			Service:        aws.String(service),
			Cluster:        aws.String(e.cluster),
			DesiredCount:   aws.Int64(count),
			TaskDefinition: aws.String(task),
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

func (e *ecsInteractor) assertColour(name string) (*ecsService, string, error) {
	nameBlue := fmt.Sprintf("%s-%s", name, BLUE)
	nameGreen := fmt.Sprintf("%s-%s", name, GREEN)

	svcBasic, errBasic := e.checkService(name)
	svcBlue, errBlue := e.checkService(nameBlue)
	svcGreen, errGreen := e.checkService(nameGreen)

	if errBasic == notFound && errBlue == notFound && errGreen == notFound {
		return nil, "", notFound
	}

	if svcBasic != nil && svcBasic.runningCount > 0 {
		return svcBasic, nameBlue, nil
	}

	if svcBlue != nil && svcBlue.runningCount > 0 {
		return svcBlue, nameGreen, nil
	}

	if svcGreen != nil && svcGreen.runningCount > 0 {
		return svcGreen, nameBlue, nil
	}

	return nil, "", colourAssign
}

func (interactor *ecsInteractor) rollingUpdate(config ecsUpdateConfig) {
	prevServiceName := config.prevService
	service, nextServiceName, err := interactor.assertColour(prevServiceName)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// update to the correct name
	prevServiceName = service.serviceName

	newService, err := interactor.checkService(nextServiceName)
	if newService.runningCount > 0 {
		fmt.Print("new service already running")
		return
	}

	newTask, err := interactor.createTask(service.taskDefinition, config.image)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	if newService.status == "INACTIVE" {
		err = interactor.createService(nextServiceName, newTask)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
	} else {
		err = interactor.updateService(nextServiceName, *newTask, 0)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
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
				err = interactor.updateService(prevServiceName, prevService.taskDefinition, 0)
				if err != nil {
					fmt.Println(err.Error())
				}
				wait.Poll(5*time.Second, config.timeout, func() (bool, error) { return interactor.waitForUpdate(prevServiceName) })
			}

			interactor.deleteService(prevServiceName)
			break
		}

		interactor.updateService(nextServiceName, nextService.taskDefinition, nextCount+1)
		wait.Poll(10*time.Second, config.timeout, func() (bool, error) { return interactor.waitForUpdate(nextServiceName) })

		interactor.updateService(prevServiceName, prevService.taskDefinition, prevCount-1)
		wait.Poll(10*time.Second, config.timeout, func() (bool, error) { return interactor.waitForUpdate(prevServiceName) })

		fmt.Printf("container started: waiting %s\n", config.updatePeriod)
		time.Sleep(config.updatePeriod)
	}
}
