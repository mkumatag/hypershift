package main

import (
	"embed"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/openshift/hypershift/releaseinfo"
	"io/ioutil"
	"os"
)

//go:embed original.json
var content embed.FS

const (
	targetRepo = "sys-powercloud-docker-local.artifactory.swg-devops.com/hypershift/ocp-release"
	imageprefix = "4.8.6-"
)

func main() {
	byteValue, err := content.ReadFile("original.json")
	if err != nil {
		os.Exit(1)
	}
	imageStream, err := releaseinfo.DeserializeImageStream(byteValue)
	if err != nil {
		os.Exit(1)
	}
	for _, tag := range imageStream.Spec.Tags {
		tag.From.Name = targetRepo+":"+imageprefix+tag.Name
	}
	bytes, err := json.Marshal(imageStream)
	if err != nil {
		fmt.Printf("failed to marshal: %v\n", err)
	}
	err = ioutil.WriteFile("patched.json", bytes, 0644)
	if err != nil {
		fmt.Printf("failed to write to file!")
	}
}