package gce

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path"
	"time"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/google-api-go-client/compute/v1"
)

type gcloudCredentialsCache struct {
	Data []gceConfig
}

type gceConfig struct {
	Credential gceCredential
	Key        gceKey
	ProjectId  string `json:"projectId"`
}

type gceCredential struct {
	ClientId     string `json:"Client_Id"`
	ClientSecret string `json:"Client_Secret"`
	RefreshToken string `json:"Refresh_Token"`
}

type gceKey struct {
	Scope string
}

// Gets the OAuth2 token from the current user's gcloud crendentials.
func getOauthToken() (*gceCredential, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	confPath := path.Join(usr.HomeDir, ".config/gcloud/credentials")
	f, err := os.Open(confPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load gcloud credentials: %q", confPath)
	}
	defer f.Close()
	cache := &gcloudCredentialsCache{}
	if err := json.NewDecoder(f).Decode(cache); err != nil {
		return nil, err
	}
	if len(cache.Data) == 0 {
		return nil, fmt.Errorf("no gcloud credentials cached in: %q", confPath)
	}
	return &cache.Data[0].Credential, nil
}

// Gets the OAuth2 token from the current user's gcloud crendentials.
func getProjectId() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	confPath := path.Join(usr.HomeDir, ".config/gcloud/credentials")
	f, err := os.Open(confPath)
	if err != nil {
		return "", fmt.Errorf("unable to load gcloud credentials: %q", confPath)
	}
	defer f.Close()
	cache := &gcloudCredentialsCache{}
	if err := json.NewDecoder(f).Decode(cache); err != nil {
		return "", err
	}
	if len(cache.Data) == 0 {
		return "", fmt.Errorf("no gcloud credentials cached in: %q", confPath)
	}
	return cache.Data[0].ProjectId, nil
}

func NewCompute() (*compute.Service, error) {
	// Get Oauth2 token.
	creds, err := getOauthToken()
	if err != nil {
		log.Fatal(err)
	}

	// OAuth2 configuration.
	oAuth2Conf := &oauth.Config{
		ClientId:     creds.ClientId,
		ClientSecret: creds.ClientSecret,
		Scope:        compute.ComputeScope,
		RedirectURL:  "oob",
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
		AccessType:   "offline",
	}
	transport := &oauth.Transport{
		Config: oAuth2Conf,
		// Make the actual request using the cached token to authenticate.
		Token:     &oauth.Token{RefreshToken: creds.RefreshToken},
		Transport: http.DefaultTransport,
	}

	err = transport.Refresh()
	if err != nil {
		return nil, err
	}
	svc, err := compute.New(transport.Client())
	if err != nil {
		return nil, err
	}
	return svc, nil
}

type gceVmManager struct {
	projectId string
	service   *compute.Service
}

func NewGceManager() (VirtualMachineManager, error) {
	projectId, err := getProjectId()
	if err != nil {
		return nil, fmt.Errorf("unable to get project id: %v", err)
	}
	if projectId == "" {
		// XXX(monnand): Wrong
		projectId = "lmctfy-prod"
	}
	fmt.Printf("project id: %v\n", projectId)
	service, err := NewCompute()
	if err != nil {
		return nil, fmt.Errorf("unable to get compute service: %v", err)
	}
	ret := &gceVmManager{
		projectId: projectId,
		service:   service,
	}
	return ret, nil
}

func getMachineType(spec *VirtualMachineSpec) string {
	return "n1-standard-2"
}

func getZone(spec *VirtualMachineSpec) string {
	return "us-central1-a"
}

func getImage(spec *VirtualMachineSpec) string {
	return "ubuntu-trusty"
}

func getDiskName(spec *VirtualMachineSpec, instanceName string) string {
	return fmt.Sprintf("disk-%v", instanceName)
}

func (self *gceVmManager) waitForOp(op *compute.Operation, zone string) error {
	op, err := self.service.ZoneOperations.Get(self.projectId, zone, op.Name).Do()
	for op.Status != "DONE" {
		time.Sleep(5 * time.Second)
		op, err = self.service.ZoneOperations.Get(self.projectId, zone, op.Name).Do()
		if err != nil {
			log.Printf("Got compute.Operation, err: %#v, %v", op, err)
		}
		if op.Status != "PENDING" && op.Status != "RUNNING" && op.Status != "DONE" {
			log.Printf("Error waiting for operation: %s\n", op)
			return errors.New(fmt.Sprintf("Bad operation: %s", op))
		}
	}
	return err
}

func (self *gceVmManager) NewMachine(spec *VirtualMachineSpec) (*VirtualMachineInfo, error) {
	/*
		cloud, err := dockercloud.NewCloudGCE(self.projectId)
		if err != nil {
			return nil, fmt.Errorf("unable to get cloud: %v", err)
		}
		name := spec.GetName()
		instance, err := cloud.CreateInstance(name, getZone(spec))
		if err != nil {
			return nil, fmt.Errorf("unable to create instance: %v", err)
		}
		info := &VirtualMachineInfo{
			Name:    name,
			Address: instance,
		}
		return info, nil
	*/
	zone := getZone(spec)
	prefix := "https://www.googleapis.com/compute/v1/projects/" + self.projectId
	machineType := getMachineType(spec)
	// /zones/us-central1-a/machineTypes/n1-standard-1
	// https://www.googleapis.com/compute/v1/projects/debian-cloud/global/images/backports-debian-7-wheezy-v20131127
	imgSrc := fmt.Sprintf("%v/global/images/%v", prefix, getImage(spec))
	// fmt.Printf("image src: %v\n", imgSrc)
	instanceName := spec.GetName()
	diskName := getDiskName(spec, instanceName)

	opt, err := self.service.Disks.Insert(self.projectId, zone, &compute.Disk{
		Name: diskName,
	}).SourceImage(imgSrc).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create disk: %v", err)
	}
	err = self.waitForOp(opt, zone)
	if err != nil {
		return nil, fmt.Errorf("unable to create disk(): %v", err)
	}
	disklink := opt.TargetLink
	instance := &compute.Instance{
		Name:        instanceName,
		Description: "virtigo instance",
		Zone:        fmt.Sprintf("%v/zones/%v", prefix, zone),
		MachineType: fmt.Sprintf("%v/zones/%v/machineTypes/%v", prefix, zone, machineType),
		NetworkInterfaces: []*compute.NetworkInterface{
			&compute.NetworkInterface{
				AccessConfigs: []*compute.AccessConfig{
					&compute.AccessConfig{Type: "ONE_TO_ONE_NAT"},
				},
				Network: prefix + "/global/networks/default",
			},
		},
		Disks: []*compute.AttachedDisk{
			{
				Boot:   true,
				Type:   "PERSISTENT",
				Mode:   "READ_WRITE",
				Source: disklink,
			},
		},
	}
	// pretty.Printf("%# v\n", instance)

	opt, err = self.service.Instances.Insert(self.projectId, zone, instance).Do()

	if err != nil {
		return nil, fmt.Errorf("unable to create vm: %v", err)
	}
	err = self.waitForOp(opt, zone)
	if err != nil {
		return nil, fmt.Errorf("unable to create vm (opt): %v", err)
	}

	info := &VirtualMachineInfo{
		Name: opt.Name,
	}
	return info, nil
}
