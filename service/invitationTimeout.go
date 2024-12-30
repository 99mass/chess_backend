package service

import (
	"sync"
	"time"
)

type InvitationTimeout struct {
	roomID    string
	timer     *time.Timer
	stopChan  chan struct{}
	mutex     sync.RWMutex
	onTimeout func()
}

func NewInvitationTimeout(roomID string, duration time.Duration, onTimeout func()) *InvitationTimeout {
	return &InvitationTimeout{
		roomID:    roomID,
		stopChan:  make(chan struct{}),
		onTimeout: onTimeout,
	}
}

func (it *InvitationTimeout) Start() {
	it.mutex.Lock()
	it.timer = time.NewTimer(10 * time.Second)
	it.mutex.Unlock()

	go func() {
		select {
		case <-it.timer.C:
			if it.onTimeout != nil {
				it.onTimeout()
			}
		case <-it.stopChan:
			if !it.timer.Stop() {
				<-it.timer.C
			}
		}
	}()
}

func (it *InvitationTimeout) Stop() {
	it.mutex.Lock()
	defer it.mutex.Unlock()

	close(it.stopChan)
	if it.timer != nil {
		it.timer.Stop()
	}
}
