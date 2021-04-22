package ocp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/coreos/gangplank/cosa"
	buildapiv1 "github.com/openshift/api/build/v1"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	// These are used to parse the OpenShift API
	buildScheme       = runtime.NewScheme()
	buildCodecFactory = serializer.NewCodecFactory(buildScheme)
	buildJSONCodec    runtime.Codec

	// API Client for OpenShift builds.
	apiBuild *buildapiv1.Build
)

func init() {
	buildJSONCodec = buildCodecFactory.LegacyCodec(buildapiv1.SchemeGroupVersion)
}

// ocpBuildClient initalizes the OpenShift Build Client API.
func ocpBuildClient() error {
	// Use the OpenShift API to parse the build meta-data.
	envVarBuild, okay := os.LookupEnv("BUILD")
	if !okay {
		return ErrNoOCPBuildSpec
	}
	cfg := &buildapiv1.Build{}
	obj, _, err := buildJSONCodec.Decode([]byte(envVarBuild), nil, cfg)
	if err != nil {
		return ErrNoOCPBuildSpec
	}
	ok := false
	apiBuild, ok = obj.(*buildapiv1.Build)
	if !ok {
		return ErrNoOCPBuildSpec
	}

	// Check to make sure that this is actually on an OpenShift build node.
	strategy := apiBuild.Spec.Strategy
	if strategy.Type != "" && strategy.Type != "Custom" {
		return fmt.Errorf("unsupported build strategy")
	}
	log.Info("Executing OpenShift custom strategy builder.")

	// Check to make sure that we have a valid contextDir
	// Almost _always_ this should be in /srv for COSA.
	cDir := apiBuild.Spec.Source.ContextDir
	if cDir != "" && cDir != "/" {
		log.Infof("Using %s as working directory.", cDir)
		cosaSrvDir = cDir
	}

	return nil
}

// These envVar's are populated by OpenShift and are part of the Custom Build Stategy API
// See https://docs.openshift.com/container-platform/4.6/builds/build-strategies.html#images-custom-builder-image-ref_build-strategies
const (
	// The container image registry to push the image to.
	outputRegistryEnvVar = "OUTPUT_REGISTRY"

	// The container image tag name for the image being built.
	outputImageEnvVar = "OUTPUT_IMAGE"

	// The path to the container registry credentials for running a podman push operation.
	pushCfgPathEnvVar = "PUSH_DOCKERCFG_PATH"
)

// getCustomBuildRegistryConfig reads the environment to construct a registryUpload struct.
func getCustomBuildRegistryConfig() *registryUpload {
	r := new(registryUpload)
	var ok bool
	r.outputRegistry, ok = os.LookupEnv(outputRegistryEnvVar)
	if !ok {
		return nil
	}
	r.outputImage, ok = os.LookupEnv(outputImageEnvVar)
	if !ok {
		return nil
	}
	r.pushCfgPath, ok = os.LookupEnv(pushCfgPathEnvVar)
	if !ok {
		return nil
	}
	return r
}

// registryUpload describes a registry to upload to
type registryUpload struct {
	outputRegistry string
	outputImage    string
	pushCfgPath    string
}

// uploadOstreeToRegistry implements the custom build strategy optional step to report the results
// to the registry as an OCI image.
func uploadOstreeToRegistry(ctx context.Context, build *cosa.Build, r *registryUpload) error {
	if build == nil {
		return errors.New("build is nil")
	}
	if r == nil {
		return errors.New("upload registry is nil")
	}

	l := log.WithFields(log.Fields{
		"registry": r.outputRegistry,
		"image":    r.outputImage,
	})

	args := []string{
		"podman", "push",
		fmt.Sprintf("--authfile=%s", r.pushCfgPath),
		fmt.Sprintf("oci-archive:%s", build.BuildArtifacts.Ostree.Path),
		fmt.Sprintf("%s/%s", r.outputRegistry, r.outputImage),
	}
	cmd := exec.CommandContext(ctx, args[0], args[:1]...)
	l.WithField("cmd", args).Info("Publishing image to registry")

	l.Info("Pushing ostree to registry")
	if err := cmd.Run(); err != nil {
		errOut, _ := cmd.CombinedOutput()
		return fmt.Errorf("upload to registry failed: %s", string(errOut))
	}

	return nil
}
