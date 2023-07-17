package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSSMClient struct {
	ssmiface.SSMAPI
	mock.Mock
}

// 正常系
func TestFetchParameterStore(t *testing.T) {
	expectedParam := "TEST_PARAMETER"
	expectedValue := "test-value123!"

	// Create a mock SSM client
	mockSvc := new(MockSSMClient)

	// Configure the mock SSM client's behavior
	mockSvc.On("GetParameter", &ssm.GetParameterInput{
		Name:           aws.String(expectedParam),
		WithDecryption: aws.Bool(true),
	}).Return(&ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{
			Value: aws.String(expectedValue),
		},
	}, nil)

	// Call the fetchParameterStore function
	value, err := fetchParameterStore(expectedParam, mockSvc)

	// Assert that the mock GetParameter method was called with the expected input
	mockSvc.AssertExpectations(t)

	// Assert the returned value matches the expected value
	assert.NoError(t, err)
	assert.Equal(t, expectedValue, value)
}

// GetParameter returns mock SSM parameter.
func (m *MockSSMClient) GetParameter(i *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	args := m.Called(i)
	output, _ := args.Get(0).(*ssm.GetParameterOutput)
	return output, args.Error(1)
}

// 異常系
func TestFetchParameterStoreError(t *testing.T) {
	expectedParam := "TEST_PARAMETER"
	// expectedValue := "test-value123!"

	errMsg := "Fetch Error"

	// Create a mock SSM client
	mockSvc := new(MockSSMClient)

	// Configure the mock SSM client's behavior
	mockSvc.On("GetParameter", &ssm.GetParameterInput{
		Name:           aws.String(expectedParam),
		WithDecryption: aws.Bool(true),
	}).Return(&ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{
			Value: aws.String(""),
		},
	}, errors.New(fmt.Sprintf(errMsg)))

	// Call the fetchParameterStore function
	value, err := fetchParameterStore(expectedParam, mockSvc)

	// Assert that the mock GetParameter method was called with the expected input
	mockSvc.AssertExpectations(t)

	// Assert the returned value matches the expected value
	assert.EqualError(t, err, errMsg)
	assert.Equal(t, errMsg, value)
}
