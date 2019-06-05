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
	"math"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/knative/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	minUserID = 0
	maxUserID = math.MaxInt32
)

var (
	reservedEnvVars = sets.NewString(
		"SINK",
	)
)

// Validate implements apis.Validatable.
func (cs *ContainerSource) Validate(ctx context.Context) *apis.FieldError {
	return cs.Spec.Validate(ctx).ViaField("spec")
}

// Validate implements apis.Validatable.
func (css *ContainerSourceSpec) Validate(ctx context.Context) *apis.FieldError {
	if css.Template != nil {
		return validatePodSpec(css.Template.Spec)
	}

	return nil
}

func validatePodSpec(ps corev1.PodSpec) *apis.FieldError {
	var errs *apis.FieldError
	volumes, err := validateVolumes(ps.Volumes)
	if err != nil {
		errs = errs.Also(err.ViaField("volumes"))
	}
	switch len(ps.Containers) {
	case 0:
		errs = errs.Also(apis.ErrMissingField("containers"))
	case 1:
		errs = errs.Also(validateContainer(ps.Containers[0], volumes).
			ViaFieldIndex("containers", 0))
	default:
		errs = errs.Also(apis.ErrMultipleOneOf("containers"))
	}

	return errs
}

func validateVolumes(vs []corev1.Volume) (sets.String, *apis.FieldError) {
	volumes := sets.NewString()
	var errs *apis.FieldError
	for i, volume := range vs {
		if volumes.Has(volume.Name) {
			errs = errs.Also((&apis.FieldError{
				Message: fmt.Sprintf("duplicate volume name %q", volume.Name),
				Paths:   []string{"name"},
			}).ViaIndex(i))
		}
		errs = errs.Also(validateVolume(volume).ViaIndex(i))
		volumes.Insert(volume.Name)
	}
	return volumes, errs
}

func validateContainer(container corev1.Container, volumes sets.String) *apis.FieldError {
	if equality.Semantic.DeepEqual(container, corev1.Container{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	// Env
	errs := validateEnv(container.Env).ViaField("env")

	// Image
	if container.Image == "" {
		errs = errs.Also(apis.ErrMissingField("image"))
	} else if _, err := name.ParseReference(container.Image, name.WeakValidation); err != nil {
		fe := &apis.FieldError{
			Message: "Failed to parse image reference",
			Paths:   []string{"image"},
			Details: fmt.Sprintf("image: %q, error: %v", container.Image, err),
		}
		errs = errs.Also(fe)
	}

	// Ports
	errs = errs.Also(validateContainerPorts(container.Ports).ViaField("ports"))

	// SecurityContext
	errs = errs.Also(validateSecurityContext(container.SecurityContext).ViaField("securityContext"))

	// VolumeMounts
	errs = errs.Also(validateVolumeMounts(container.VolumeMounts, volumes).ViaField("volumeMounts"))

	return errs
}

func validateVolume(volume corev1.Volume) *apis.FieldError {
	var errs *apis.FieldError
	if volume.Name == "" {
		errs = apis.ErrMissingField("name")
	} else if len(validation.IsDNS1123Label(volume.Name)) != 0 {
		errs = apis.ErrInvalidValue(volume.Name, "name")
	}

	vs := volume.VolumeSource
	if vs.Secret == nil && vs.ConfigMap == nil {
		errs = errs.Also(apis.ErrMissingOneOf("secret", "configMap"))
	}

	return errs
}

func validateEnv(envVars []corev1.EnvVar) *apis.FieldError {
	var errs *apis.FieldError
	for i, env := range envVars {
		errs = errs.Also(validateEnvVar(env).ViaIndex(i))
	}
	return errs
}

func validateEnvVar(env corev1.EnvVar) *apis.FieldError {
	var errs *apis.FieldError
	if env.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	} else if reservedEnvVars.Has(env.Name) {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("%q is a reserved environment variable", env.Name),
			Paths:   []string{"name"},
		})
	}

	return errs
}

func validateVolumeMounts(mounts []corev1.VolumeMount, volumes sets.String) *apis.FieldError {
	var errs *apis.FieldError
	// Check that volume mounts match names in "volumes", that "volumes" has 100%
	// coverage, and the field restrictions.
	seenName := sets.NewString()
	seenMountPath := sets.NewString()
	for i, vm := range mounts {
		// This effectively checks that Name is non-empty because Volume name must be non-empty.
		if !volumes.Has(vm.Name) {
			errs = errs.Also((&apis.FieldError{
				Message: "volumeMount has no matching volume",
				Paths:   []string{"name"},
			}).ViaIndex(i))
		}
		seenName.Insert(vm.Name)

		if vm.MountPath == "" {
			errs = errs.Also(apis.ErrMissingField("mountPath").ViaIndex(i))
		} else if !filepath.IsAbs(vm.MountPath) {
			errs = errs.Also(apis.ErrInvalidValue(vm.MountPath, "mountPath").ViaIndex(i))
		} else if seenMountPath.Has(filepath.Clean(vm.MountPath)) {
			errs = errs.Also(apis.ErrInvalidValue(
				fmt.Sprintf("%q must be unique", vm.MountPath), "mountPath").ViaIndex(i))
		}
		seenMountPath.Insert(filepath.Clean(vm.MountPath))
	}

	if missing := volumes.Difference(seenName); missing.Len() > 0 {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("volumes not mounted: %v", missing.List()),
			Paths:   []string{apis.CurrentField},
		})
	}
	return errs
}

func validateContainerPorts(ports []corev1.ContainerPort) *apis.FieldError {
	if len(ports) == 0 {
		return nil
	}

	var errs *apis.FieldError
	for _, userPort := range ports {
		if userPort.ContainerPort < 1 || userPort.ContainerPort > 65535 {
			errs = errs.Also(apis.ErrOutOfBoundsValue(userPort.ContainerPort,
				1, 65535, "containerPort"))
		}
	}

	return errs
}

func validateSecurityContext(sc *corev1.SecurityContext) *apis.FieldError {
	if sc == nil {
		return nil
	}

	var errs *apis.FieldError
	if sc.RunAsUser != nil {
		uid := *sc.RunAsUser
		if uid < minUserID || uid > maxUserID {
			errs = errs.Also(apis.ErrOutOfBoundsValue(uid, minUserID, maxUserID, "runAsUser"))
		}
	}
	return errs
}
