// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"github.com/concourse/atc/exec"
)

type FakeSuccessIndicator struct {
	SuccessfulStub        func() bool
	successfulMutex       sync.RWMutex
	successfulArgsForCall []struct{}
	successfulReturns struct {
		result1 bool
	}
}

func (fake *FakeSuccessIndicator) Successful() bool {
	fake.successfulMutex.Lock()
	fake.successfulArgsForCall = append(fake.successfulArgsForCall, struct{}{})
	fake.successfulMutex.Unlock()
	if fake.SuccessfulStub != nil {
		return fake.SuccessfulStub()
	} else {
		return fake.successfulReturns.result1
	}
}

func (fake *FakeSuccessIndicator) SuccessfulCallCount() int {
	fake.successfulMutex.RLock()
	defer fake.successfulMutex.RUnlock()
	return len(fake.successfulArgsForCall)
}

func (fake *FakeSuccessIndicator) SuccessfulReturns(result1 bool) {
	fake.SuccessfulStub = nil
	fake.successfulReturns = struct {
		result1 bool
	}{result1}
}

var _ exec.SuccessIndicator = new(FakeSuccessIndicator)
