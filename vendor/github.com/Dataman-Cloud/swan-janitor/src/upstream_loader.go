package janitor

import (
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
)

const (
	UPSTREAM_LOADER_KEY = "UpstreamLoader"
)

type TargetChangeEvent struct {
	Change string // "add" or "del" or "update"

	AppID    string
	TaskID   string
	TaskIP   string
	TaskPort uint32
	PortName string

	VersionID  string
	AppVersion string

	Weight float64
}

type UpstreamLoader struct {
	Upstreams []*Upstream `json:"upstreams"`

	eventChan chan *TargetChangeEvent
	mu        sync.RWMutex
}

func NewUpstreamLoader(eventChan chan *TargetChangeEvent) *UpstreamLoader {
	UpstreamLoader := &UpstreamLoader{
		Upstreams: make([]*Upstream, 0),
		eventChan: eventChan,
	}

	return UpstreamLoader
}

func (loader *UpstreamLoader) Start(ctx context.Context) error {
	log.Debug("UpstreamLoader starts listening app event...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case targetChangeEvent := <-loader.eventChan:
			log.Debugf("upstreamLoader receive app event: %+v", targetChangeEvent)

			upstream := upstreamFromChangeEvent(targetChangeEvent)
			target := targetFromChangeEvent(targetChangeEvent)

			if strings.ToLower(targetChangeEvent.Change) == "add" {
				if loader.Contains(upstream) {
					u := loader.Get(upstream.AppID)
					// add target if not exists
					if !u.ContainsTarget(target.TaskID) {
						u.AddTarget(target)
					}
				} else {
					upstream.AddTarget(target)
					loader.Upstreams = append(loader.Upstreams, upstream)
				}
			}

			if strings.ToLower(targetChangeEvent.Change) == "del" {
				if loader.Contains(upstream) {
					u := loader.Get(upstream.AppID)

					u.RemoveTarget(target)
					if len(u.Targets) == 0 { // remove upstream after last target was removed
						loader.RemoveUpstream(upstream)
					}
				}
			}

			// targetChangeEvent update, weight only for the time present
			if strings.ToLower(targetChangeEvent.Change) == "change" {
				if loader.Contains(upstream) {
					u := loader.Get(upstream.AppID)
					if u == nil {
						log.Errorf("failed to find upstream %s from loader", upstream.AppID)
						break
					}

					t := u.GetTarget(target.TaskID)
					if t == nil {
						log.Errorf("failed to find target %s from upstream", target.TaskID)
						break
					}

					u.UpdateTargetWeight(target.TaskID, target.Weight)
				}
			}
		}
	}
}

func (loader *UpstreamLoader) Contains(newUpstream *Upstream) bool {
	for _, u := range loader.Upstreams {
		if u.Equal(newUpstream) {
			return true
		}
	}

	return false
}

func (loader *UpstreamLoader) RemoveUpstream(upstream *Upstream) {
	index := -1
	for k, v := range loader.Upstreams {
		if v.Equal(upstream) {
			index = k
			break
		}
	}

	if index >= 0 {
		loader.mu.Lock()
		defer loader.mu.Unlock()
		loader.Upstreams = append(loader.Upstreams[:index], loader.Upstreams[index+1:]...)
	}
}

func (loader *UpstreamLoader) List() []*Upstream {
	return loader.Upstreams
}

func (loader *UpstreamLoader) Get(appID string) *Upstream {
	loader.mu.RLock()
	defer loader.mu.RUnlock()

	for _, u := range loader.Upstreams {
		if formattedAppId(u.AppID) == formattedAppId(appID) {
			return u
		}
	}

	return nil
}

func targetFromChangeEvent(targetChangeEvent *TargetChangeEvent) *Target {
	return &Target{
		AppID:      targetChangeEvent.AppID,
		TaskID:     targetChangeEvent.TaskID,
		TaskIP:     targetChangeEvent.TaskIP,
		TaskPort:   targetChangeEvent.TaskPort,
		PortName:   targetChangeEvent.PortName,
		VersionID:  targetChangeEvent.VersionID,
		AppVersion: targetChangeEvent.AppVersion,
		Weight:     targetChangeEvent.Weight,
	}
}

func upstreamFromChangeEvent(targetChangeEvent *TargetChangeEvent) *Upstream {
	up := NewUpstream()
	up.Targets = make([]*Target, 0)
	up.AppID = targetChangeEvent.AppID

	return up
}

func formattedAppId(appID string) string {
	return strings.ToLower(strings.Replace(appID, "-", ".", -1))
}
