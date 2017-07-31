package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dataman-Cloud/swan/src/manager/connector"
	"github.com/Dataman-Cloud/swan/src/mesosproto/mesos"
	"github.com/Dataman-Cloud/swan/src/mesosproto/sched"
	"github.com/Dataman-Cloud/swan/src/types"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	uuid "github.com/satori/go.uuid"
)

type Task struct {
	ID      string
	Version *types.Version
	Slot    *Slot

	State  string
	Stdout string
	Stderr string

	HostPorts     []uint64
	OfferID       string
	AgentID       string
	Ip            string
	AgentHostName string

	Reason  string // mesos updateStatus field
	Message string // mesos updateStatus field
	Source  string // mesos updateStatus field

	ContainerId   string
	ContainerName string

	Created     time.Time
	ArchivedAt  time.Time
	taskBuilder *TaskBuilder
}

func NewTask(version *types.Version, slot *Slot) *Task {
	task := &Task{
		ID:        fmt.Sprintf("%s-%s", slot.ID, strings.Replace(uuid.NewV4().String(), "-", "", -1)),
		Version:   version,
		Slot:      slot,
		Ip:        slot.Ip,
		HostPorts: make([]uint64, 0),
		Created:   time.Now(),
	}

	return task
}

func (task *Task) PrepareTaskInfo(ow *OfferWrapper) *mesos.TaskInfo {
	defaultLabels := make(map[string]string)
	defaultLabels["DM_USER"] = task.Slot.Version.RunAs
	defaultLabels["DM_CLUSTER"] = task.Slot.App.ClusterID
	defaultLabels["DM_SLOT_INDEX"] = strconv.Itoa(task.Slot.Index)
	defaultLabels["DM_SLOT_ID"] = task.Slot.ID
	defaultLabels["DM_TASK_ID"] = task.ID
	defaultLabels["DM_APP_NAME"] = task.Slot.App.Name
	defaultLabels["DM_APP_ID"] = task.Slot.App.ID

	offer := ow.Offer
	logrus.Infof("Prepared task %s for launch with offer %s", task.Slot.ID, *offer.GetId().Value)

	versionSpec := task.Slot.Version
	containerSpec := task.Slot.Version.Container
	dockerSpec := task.Slot.Version.Container.Docker

	task.taskBuilder = NewTaskBuilder(task)
	task.taskBuilder.SetName(task.Slot.ID).SetTaskId(task.ID).SetAgentId(*offer.GetAgentId().Value)
	task.taskBuilder.SetResources(task.Slot.ResourcesNeeded())
	task.taskBuilder.SetCommand(task.Slot.Version.Command, task.Slot.Version.Args)

	task.taskBuilder.SetContainerType("docker").SetContainerDockerImage(dockerSpec.Image).
		SetContainerDockerPrivileged(dockerSpec.Privileged).
		SetContainerDockerForcePullImage(dockerSpec.ForcePullImage).
		AppendContainerDockerVolumes(containerSpec.Volumes)

	task.taskBuilder.AppendContainerDockerEnvironments(versionSpec.Env).SetURIs(versionSpec.URIs).AppendTaskInfoLabels(versionSpec.Labels)
	task.taskBuilder.AppendTaskInfoLabels(defaultLabels)

	task.taskBuilder.AppendContainerDockerParameters(task.Slot.Version.Container.Docker.Parameters)

	if task.Slot.App.IsFixed() {
		ipParameter := types.Parameter{
			Key:   "ip",
			Value: task.Slot.Ip,
		}
		task.taskBuilder.AppendContainerDockerParameters([]*types.Parameter{&ipParameter})
	}

	for k, v := range defaultLabels {
		p := types.Parameter{
			Key:   "label",
			Value: fmt.Sprintf("%s=%s", k, v),
		}
		task.taskBuilder.AppendContainerDockerParameters([]*types.Parameter{&p})
	}

	task.taskBuilder.SetNetwork(dockerSpec.Network, ow.PortsRemain())
	if versionSpec.HealthCheck != nil {
		task.taskBuilder.SetHealthCheck(versionSpec.HealthCheck)
	}
	task.HostPorts = task.taskBuilder.HostPorts

	return task.taskBuilder.taskInfo
}

func (task *Task) Kill() {
	logrus.Infof("Kill task %s", task.Slot.ID)
	call := &sched.Call{
		FrameworkId: connector.Instance().FrameworkInfo.GetId(),
		Type:        sched.Call_KILL.Enum(),
		Kill: &sched.Call_Kill{
			TaskId: &mesos.TaskID{
				Value: proto.String(task.ID),
			},
			AgentId: &mesos.AgentID{
				Value: &task.AgentID,
			},
		},
	}

	if task.Version.KillPolicy != nil {
		if task.Version.KillPolicy.Duration != 0 {
			call.Kill.KillPolicy = &mesos.KillPolicy{
				GracePeriod: &mesos.DurationInfo{
					Nanoseconds: proto.Int64(task.Version.KillPolicy.Duration * 1000 * 1000),
				},
			}
		}
	}

	connector.Instance().SendCall(call)
}
