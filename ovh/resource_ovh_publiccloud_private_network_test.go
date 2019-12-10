package ovh

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
)

var testAccPublicCloudPrivateNetworkConfig_attachVrack = `
resource "ovh_vrack_cloudproject" "attach" {
  vrack_id   = "%s"
  project_id = "%s"
}

data "ovh_cloud_regions" "regions" {
  project_id = ovh_vrack_cloudproject.attach.project_id

  has_services_up = ["network"]
}
`

var testAccPublicCloudPrivateNetworkConfig_noAttachVrack = `
data "ovh_cloud_regions" "regions" {
  project_id = "%s"

  has_services_up = ["network"]
}
`

var testAccPublicCloudPrivateNetworkConfig_basic = `
%s

resource "ovh_cloud_network_private" "network" {
  project_id = data.ovh_cloud_regions.regions.project_id
  vlan_id    = 0
  name       = "terraform_testacc_private_net"
  regions    = tolist(data.ovh_cloud_regions.regions.names)
}
`

func testAccPublicCloudPrivateNetworkConfig() string {
	attachVrack := fmt.Sprintf(
		testAccPublicCloudPrivateNetworkConfig_attachVrack,
		os.Getenv("OVH_VRACK"),
		os.Getenv("OVH_PUBLIC_CLOUD"),
	)
	noAttachVrack := fmt.Sprintf(
		testAccPublicCloudPrivateNetworkConfig_noAttachVrack,
		os.Getenv("OVH_PUBLIC_CLOUD"),
	)

	if os.Getenv("OVH_ATTACH_VRACK") == "0" {
		return fmt.Sprintf(
			testAccPublicCloudPrivateNetworkConfig_basic,
			noAttachVrack,
		)
	}

	return fmt.Sprintf(
		testAccPublicCloudPrivateNetworkConfig_basic,
		attachVrack,
	)
}

func init() {
	resource.AddTestSweepers("ovh_cloud_network_private", &resource.Sweeper{
		Name: "ovh_cloud_network_private",
		F:    testSweepCloudNetworkPrivate,
	})
}

func testSweepCloudNetworkPrivate(region string) error {
	client, err := sharedClientForRegion(region)
	if err != nil {
		return fmt.Errorf("error getting client: %s", err)
	}

	vrack := os.Getenv("OVH_VRACK")
	if vrack == "" {
		return fmt.Errorf("OVH_VRACK must be set")
	}

	projectId := os.Getenv("OVH_PUBLIC_CLOUD")
	if projectId == "" {
		return fmt.Errorf("OVH_PUBLIC_CLOUD must be set")
	}

	networkIds := []string{}
	err = client.Get(fmt.Sprintf("/cloud/project/%s/network/private", projectId), &networkIds)
	if err != nil {
		return fmt.Errorf("error listing private networks for project %q:\n\t %q", projectId, err)
	}

	for _, n := range networkIds {
		r := &PublicCloudPrivateNetworkResponse{}
		err = client.Get(fmt.Sprintf("/cloud/project/%s/network/private/%s", projectId, n), r)
		if err != nil {
			return fmt.Errorf("error getting private network %q for project %q:\n\t %q", n, projectId, err)
		}

		if !strings.HasPrefix(r.Name, test_prefix) {
			continue
		}

		log.Printf("[DEBUG] found dangling network & subnets for project: %s, id: %s", projectId, n)
		err = resource.Retry(5*time.Minute, func() *resource.RetryError {
			subnetIds := []string{}
			err = client.Get(fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", projectId, n), &subnetIds)
			if err != nil {
				return resource.RetryableError(fmt.Errorf("error listing private network subnets for project %q:\n\t %q", projectId, err))
			}

			for _, s := range subnetIds {
				if err := client.Delete(fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet/%s", projectId, n, s), nil); err != nil {
					return resource.RetryableError(err)
				}
			}

			if err := client.Delete(fmt.Sprintf("/cloud/project/%s/network/private/%s", projectId, n), nil); err != nil {
				return resource.RetryableError(err)
			}

			// Successful cascade delete
			log.Printf("[DEBUG] successful cascade delete of network & subnets for project: %s, id: %s", projectId, n)
			return nil
		})

		if err != nil {
			return err
		}

	}

	return nil
}

func TestAccPublicCloudPrivateNetwork_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccCheckPublicCloudPrivateNetworkPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccPublicCloudPrivateNetworkConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("ovh_cloud_network_private.network", "project_id"),
					resource.TestCheckResourceAttrSet("ovh_cloud_network_private.network", "id"),
					resource.TestCheckResourceAttr("ovh_cloud_network_private.network", "vlan_id", "0"),
				),
			},
		},
	})
}

func testAccCheckPublicCloudPrivateNetworkPreCheck(t *testing.T) {
	testAccPreCheckPublicCloud(t)
	testAccCheckPublicCloudExists(t)
	testAccPreCheckVRack(t)
}
