package provision

import (
	"fmt"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/provision/pkgaction"
	"github.com/docker/machine/libmachine/provision/serviceaction"
)

const (
	hostnameTmpl = `sudo mkdir -p /var/lib/rancher/conf/cloud-config.d/  
sudo tee /var/lib/rancher/conf/cloud-config.d/machine-hostname.yml << EOF
#cloud-config

hostname: %s
EOF
`
)

func init() {
	Register("RancherOS", &RegisteredProvisioner{
		New: NewRancherProvisioner,
	})
}

func NewRancherProvisioner(d drivers.Driver) Provisioner {
	return &RancherProvisioner{
		GenericProvisioner{
			SSHCommander:      GenericSSHCommander{Driver: d},
			DockerOptionsDir:  "/var/lib/rancher/conf",
			DaemonOptionsFile: "/var/lib/rancher/conf/docker",
			OsReleaseID:       "rancheros",
			Driver:            d,
		},
	}
}

type RancherProvisioner struct {
	GenericProvisioner
}

func (provisioner *RancherProvisioner) String() string {
	return "rancheros"
}

func (provisioner *RancherProvisioner) Service(name string, action serviceaction.ServiceAction) error {
	command := fmt.Sprintf("sudo system-docker %s %s", action.String(), name)

	if _, err := provisioner.SSHCommand(command); err != nil {
		return err
	}

	return nil
}

func (provisioner *RancherProvisioner) Package(name string, action pkgaction.PackageAction) error {
	var packageAction string

	if name == "docker" && action == pkgaction.Upgrade {
		return provisioner.upgrade()
	}

	switch action {
	case pkgaction.Install:
		packageAction = "enabled"
	case pkgaction.Remove:
		packageAction = "disable"
	case pkgaction.Upgrade:
		// TODO: support upgrade
		packageAction = "upgrade"
	}

	command := fmt.Sprintf("sudo rancherctl service %s %s", packageAction, name)

	if _, err := provisioner.SSHCommand(command); err != nil {
		return err
	}

	return nil
}

func (provisioner *RancherProvisioner) Provision(engineOptions engine.Options) error {
	log.Debugf("Running RancherOS provisioner on %s", provisioner.Driver.GetMachineName())

	provisioner.EngineOptions = engineOptions

	if provisioner.EngineOptions.StorageDriver == "" {
		provisioner.EngineOptions.StorageDriver = "overlay"
	} else if provisioner.EngineOptions.StorageDriver != "overlay" {
		return fmt.Errorf("Unsupported storage driver: %s", provisioner.EngineOptions.StorageDriver)
	}

	log.Debugf("Setting hostname %s", provisioner.Driver.GetMachineName())
	if err := provisioner.SetHostname(provisioner.Driver.GetMachineName()); err != nil {
		return err
	}

	for _, pkg := range provisioner.Packages {
		log.Debugf("Installing package %s", pkg)
		if err := provisioner.Package(pkg, pkgaction.Install); err != nil {
			return err
		}
	}

	if engineOptions.InstallURL != drivers.DefaultEngineInstallURL {
		log.Debugf("Selecting docker engine: %s", engineOptions.InstallURL)
		return selectDocker(provisioner, engineOptions.InstallURL)
	}

	log.Debugf("Skipping docker engine default: %s", engineOptions.InstallURL)
	return nil
}

func (provisioner *RancherProvisioner) SetHostname(hostname string) error {
	// /etc/hosts is bind mounted from Docker, this is hack to that the generic provisioner doesn't try to mv /etc/hosts
	if _, err := provisioner.SSHCommand("sed /127.0.1.1/d /etc/hosts > /tmp/hosts && cat /tmp/hosts | sudo tee /etc/hosts"); err != nil {
		return err
	}

	if err := provisioner.GenericProvisioner.SetHostname(hostname); err != nil {
		return err
	}

	if _, err := provisioner.SSHCommand(fmt.Sprintf(hostnameTmpl, hostname)); err != nil {
		return err
	}

	return nil
}

func (provisioner *RancherProvisioner) upgrade() error {
	log.Infof("Running upgrade")
	if _, err := provisioner.SSHCommand("sudo rancherctl os upgrade -f --no-reboot"); err != nil {
		return err
	}

	log.Infof("Upgrade succeeded, rebooting")
	// ignore errors here because the SSH connection will close
	provisioner.SSHCommand("sudo reboot")

	return nil
}

func selectDocker(p Provisioner, baseURL string) error {
	// TODO: detect if its a cloud-init, or a ros setting - and use that..
	if output, err := p.SSHCommand(fmt.Sprintf("wget -O- %s | sh -", baseURL)); err != nil {
		return fmt.Errorf("error selecting docker: (%s) %s", err, output)
	}

	return nil
}
