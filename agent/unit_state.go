package agent

import (
	"sync"
	"time"

	log "github.com/coreos/fleet/Godeps/_workspace/src/github.com/golang/glog"

	"github.com/coreos/fleet/machine"
	"github.com/coreos/fleet/registry"
	"github.com/coreos/fleet/unit"
)

func NewUnitStatePublisher(mgr unit.UnitManager, reg registry.Registry, mach machine.Machine) *UnitStatePublisher {
	cache := make(map[string]*unit.UnitState)
	return &UnitStatePublisher{mgr, reg, mach, sync.RWMutex{}, cache}
}

type UnitStatePublisher struct {
	mgr  unit.UnitManager
	reg  registry.Registry
	mach machine.Machine

	mutex sync.RWMutex
	cache map[string]*unit.UnitState
}

// Run caches all of the heartbeat objects from the provided channel, publishing
// them to the Registry every 5s. Heartbeat objects are also published as they
// are received on the channel.
func (p *UnitStatePublisher) Run(beatchan <-chan *unit.UnitStateHeartbeat, stop chan bool) {
	go func() {
		tick := time.Tick(5 * time.Second)
		for {
			select {
			case <-stop:
				return
			case <-tick:
				p.PublishAll()
			}
		}
	}()

	for {
		select {
		case <-stop:
			return
		case bt := <-beatchan:
			p.addToCache(bt)
		}
	}
}

func (p *UnitStatePublisher) PublishAll() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for name, us := range p.cache {
		p.publishOne(name, us)
	}

	p.cache = make(map[string]*unit.UnitState)
}

func (p *UnitStatePublisher) publishOne(name string, us *unit.UnitState) {
	if us == nil {
		log.Infof("Destroying UnitState(%s) in Registry", name)
		err := p.reg.RemoveUnitState(name)
		if err != nil {
			log.Errorf("Failed to destroy UnitState(%s) in Registry: %v", name, err)
		}
	} else {
		us.MachineID = p.mach.State().ID
		log.Infof("Pushing UnitState(%s) to Registry: loadState=%s, activeState=%s, subState=%s", name, us.LoadState, us.ActiveState, us.SubState)
		p.reg.SaveUnitState(name, us)
	}
}

func (p *UnitStatePublisher) addToCache(update *unit.UnitStateHeartbeat) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	last, existed := p.cache[update.Name]
	p.cache[update.Name] = update.State

	// As an optimization, publish changes as they flow in
	if existed && last != update.State {
		go p.publishOne(update.Name, update.State)
	}
}