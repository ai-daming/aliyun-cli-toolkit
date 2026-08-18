package oss

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mamamate/aliyun-cli-toolkit/internal/media"
)

type sessionPolicy struct {
	Version   string            `json:"Version"`
	Statement []policyStatement `json:"Statement"`
}

type policyStatement struct {
	Effect    string           `json:"Effect"`
	Action    []string         `json:"Action"`
	Resource  []string         `json:"Resource"`
	Condition *policyCondition `json:"Condition,omitempty"`
}

type policyCondition struct {
	StringEquals map[string]string `json:"StringEquals"`
}

func buildObjectPolicy(bucket, action, objectKey string) (string, error) {
	if err := media.ValidateBucket(bucket); err != nil {
		return "", err
	}
	if err := media.ValidateObjectInput(objectKey); err != nil {
		return "", err
	}
	if action != "oss:PutObject" && action != "oss:GetObject" && action != "oss:DeleteObject" {
		return "", errors.New("unsupported object policy action")
	}
	return encodePolicy(policyStatement{
		Effect:   "Allow",
		Action:   []string{action},
		Resource: []string{fmt.Sprintf("acs:oss:*:*:%s/%s", bucket, objectKey)},
	})
}

func buildListPolicy(bucket, prefix string) (string, error) {
	if err := media.ValidateBucket(bucket); err != nil {
		return "", err
	}
	if err := media.ValidateObjectInput(prefix); err != nil {
		return "", err
	}
	return encodePolicy(policyStatement{
		Effect:   "Allow",
		Action:   []string{"oss:ListObjects"},
		Resource: []string{fmt.Sprintf("acs:oss:*:*:%s", bucket)},
		Condition: &policyCondition{StringEquals: map[string]string{
			"oss:Prefix": prefix,
		}},
	})
}

func encodePolicy(statement policyStatement) (string, error) {
	encoded, err := json.Marshal(sessionPolicy{
		Version:   "1",
		Statement: []policyStatement{statement},
	})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
