/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"
	"github.com/knative/pkg/apis"
)

func (c *KafkaChannel) Validate(ctx context.Context) *apis.FieldError {
	return c.Spec.Validate(ctx).ViaField("spec")
}

func (cs *KafkaChannelSpec) Validate(ctx context.Context) *apis.FieldError {
	var errs *apis.FieldError

	if cs.NumPartitions <= 0 {
		fe := apis.ErrInvalidValue(cs.NumPartitions, "numPartitions")
		errs = errs.Also(fe)
	}

	if cs.ReplicationFactor <= 0 {
		fe := apis.ErrInvalidValue(cs.ReplicationFactor, "replicationFactor")
		errs = errs.Also(fe)
	}

	if cs.Subscribable != nil {
		for i, subscriber := range cs.Subscribable.Subscribers {
			if subscriber.ReplyURI == "" && subscriber.SubscriberURI == "" {
				fe := apis.ErrMissingField("replyURI", "subscriberURI")
				fe.Details = "expected at least one of, got none"
				errs = errs.Also(fe.ViaField(fmt.Sprintf("subscriber[%d]", i)).ViaField("subscribable"))
			}
		}
	}
	return errs
}
