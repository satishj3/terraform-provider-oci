// Copyright (c) 2017, 2019, Oracle and/or its affiliates. All rights reserved.

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/oracle/oci-go-sdk/database"
	"github.com/stretchr/testify/suite"

	oci_common "github.com/oracle/oci-go-sdk/common"
)

var (
	DbSystemResourceConfig = SubnetResourceConfig + `
resource "oci_database_db_system" "test_db_system" {
	availability_domain = "${oci_core_subnet.test_subnet.availability_domain}"
	compartment_id = "${var.compartment_id}"
	subnet_id = "${oci_core_subnet.test_subnet.id}"
	database_edition = "ENTERPRISE_EDITION"
	disk_redundancy = "NORMAL"
	shape = "BM.DenseIO2.52"
	cpu_core_count = "2"
	ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
	domain = "${oci_core_subnet.test_subnet.subnet_domain_name}"
	hostname = "myOracleDB"
	data_storage_size_in_gb = "256"
	license_model = "LICENSE_INCLUDED"
	node_count = "1"
	display_name = "tfDbSystemTest"
	db_home {
		db_version = "12.1.0.2"
		display_name = "dbHome1"
		database {
			"admin_password" = "BEstrO0ng_#11"
			"db_name" = "tfDbName"
		}
	}
}

data "oci_database_db_homes" "t" {
	compartment_id = "${var.compartment_id}"
	db_system_id = "${oci_database_db_system.test_db_system.id}"
	filter {
		name = "display_name"
		values = ["dbHome1"]
	}
}
`
)

type ResourceDatabaseDBSystemTestSuite struct {
	suite.Suite
	Providers    map[string]terraform.ResourceProvider
	Config       string
	ResourceName string
	Token        string
	TokenFn      TokenFn
}

func (s *ResourceDatabaseDBSystemTestSuite) SetupTest() {
	s.Token, s.TokenFn = tokenize()
	s.Providers = testAccProviders
	testAccPreCheck(s.T())
	s.Config = legacyTestProviderConfig() + `
	data "oci_identity_availability_domains" "ADs" {
		compartment_id = "${var.compartment_id}"
	}

	resource "oci_core_virtual_network" "t" {
		compartment_id = "${var.compartment_id}"
		cidr_block = "10.1.0.0/16"
		display_name = "-tf-vcn"
		dns_label = "tfvcn"
	}

	resource "oci_core_subnet" "t" {
		availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
		cidr_block          = "10.1.20.0/24"
		display_name        = "TFSubnet1"
		compartment_id      = "${var.compartment_id}"
		vcn_id              = "${oci_core_virtual_network.t.id}"
		route_table_id      = "${oci_core_virtual_network.t.default_route_table_id}"
		dhcp_options_id     = "${oci_core_virtual_network.t.default_dhcp_options_id}"
		security_list_ids   = ["${oci_core_virtual_network.t.default_security_list_id}"]
		dns_label           = "tfsubnet"
	}
	resource "oci_core_subnet" "t2" {
		availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
		cidr_block          = "10.1.21.0/24"
		display_name        = "TFSubnet2"
		compartment_id      = "${var.compartment_id}"
		vcn_id              = "${oci_core_virtual_network.t.id}"
		route_table_id      = "${oci_core_virtual_network.t.default_route_table_id}"
		dhcp_options_id     = "${oci_core_virtual_network.t.default_dhcp_options_id}"
		security_list_ids   = ["${oci_core_virtual_network.t.default_security_list_id}"]
		dns_label           = "tfsubnet2"
	}`
	s.ResourceName = "oci_database_db_system.t"
}

func dbWaitCondition(response oci_common.OCIOperationResponse) bool {
	if listBackupResponse, ok := response.Response.(database.ListBackupsResponse); ok {
		if len(listBackupResponse.Items) > 0 {
			return listBackupResponse.Items[0].LifecycleState != database.BackupSummaryLifecycleStateActive
		}
		return true
	}
	return false
}

func dbResponseFetchOperation(client *OracleClients, resourceId *string, retryPolicy *oci_common.RetryPolicy) error {
	_, err := client.databaseClient.ListBackups(context.Background(), database.ListBackupsRequest{
		DatabaseId: resourceId,
		RequestMetadata: oci_common.RequestMetadata{
			RetryPolicy: retryPolicy,
		},
	})
	return err
}

func (s *ResourceDatabaseDBSystemTestSuite) TestAccResourceDatabaseDBSystemFromBackup() {
	if strings.Contains(getEnvSettingWithBlankDefault("suppressed_tests"), "DBSystem_basic") {
		s.T().Skip("Skipping subset dbsystem test.")
	}

	const DBWaitConditionDuration = time.Duration(20 * time.Minute)
	const DataBaseSystemWithBackup = `
	resource "oci_database_db_system" "test_db_system" {
		availability_domain = "${oci_core_subnet.test_subnet.availability_domain}"
		compartment_id = "${var.compartment_id}"
		subnet_id = "${oci_core_subnet.test_subnet.id}"
		database_edition = "ENTERPRISE_EDITION"
		disk_redundancy = "NORMAL"
		shape = "BM.DenseIO2.52"
		cpu_core_count = "2"
		ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
		domain = "${oci_core_subnet.test_subnet.subnet_domain_name}"
		hostname = "myOracleDB"
		data_storage_size_in_gb = "256"
		license_model = "LICENSE_INCLUDED"
		node_count = "1"
		display_name = "tfDbSystemTest"
		db_home {
			db_version = "12.1.0.2"
			display_name = "dbHome1"
			database {
				"admin_password" = "BEstrO0ng_#11"
				"db_name" = "aTFdb"
				character_set = "AL32UTF8"
				ncharacter_set = "AL16UTF16"
				db_workload = "OLTP"
				pdb_name = "pdbName"
				db_backup_config {
					auto_backup_enabled = true
				}
			}
		}
	}
	data "oci_database_db_homes" "t" {
		compartment_id = "${var.compartment_id}"
		db_system_id = "${oci_database_db_system.test_db_system.id}"
		filter {
			name = "display_name"
			values = ["dbHome1"]
		}
	}`

	var resId string

	resource.Test(s.T(), resource.TestCase{
		Providers: s.Providers,
		Steps: []resource.TestStep{
			// create
			{
				Config: s.Config + SubnetResourceConfig + DataBaseSystemWithBackup + `
				data "oci_database_databases" "t" {
  					compartment_id = "${var.compartment_id}"
  					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.id}"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) (err error) {
						resId, err = fromInstanceState(s, "data.oci_database_databases.t", "databases.0.id")
						return err
					},
				),
			},
			// wait for backup and create new db from it
			{
				PreConfig: waitTillCondition(testAccProvider, &resId, dbWaitCondition, DBWaitConditionDuration,
					dbResponseFetchOperation, "core", false),
				Config: s.Config + SubnetResourceConfig + DataBaseSystemWithBackup + `
				data "oci_database_databases" "t" {
  					compartment_id = "${var.compartment_id}"
  					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.id}"
				}
				data "oci_database_backups" "test_backups"{
					database_id = "${data.oci_database_databases.t.databases.0.id}"

				}
				resource "oci_database_db_system" "t" {
					availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
					compartment_id = "${var.compartment_id}"
					subnet_id = "${oci_core_subnet.t.id}"
					database_edition = "ENTERPRISE_EDITION"
					disk_redundancy = "NORMAL"
					shape = "BM.DenseIO2.52"
					cpu_core_count = "2"
					ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
					domain = "${oci_core_subnet.t.dns_label}.${oci_core_virtual_network.t.dns_label}.oraclevcn.com"
					hostname = "myOracleDB"
					data_storage_size_in_gb = "256"
					license_model = "LICENSE_INCLUDED"
					node_count = "1"
					display_name = "tfDbSystemTestFromBackup"
					source = "DB_BACKUP"
					db_home {
						db_version = "12.1.0.2"
						database {
							"admin_password" = "BEstrO0ng_#11"
							backup_tde_password = "BEstrO0ng_#11"
							"backup_id" = "${data.oci_database_backups.test_backups.backups.0.id}"
							"db_name" = "dbback"
						}
					}
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// DB System Resource tests
					resource.TestCheckResourceAttrSet(s.ResourceName, "id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "availability_domain"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "compartment_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "subnet_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "time_created"),
					resource.TestCheckResourceAttr(s.ResourceName, "database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr(s.ResourceName, "disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr(s.ResourceName, "shape", "BM.DenseIO2.52"),
					resource.TestCheckResourceAttr(s.ResourceName, "cpu_core_count", "2"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestCheckResourceAttr(s.ResourceName, "display_name", "tfDbSystemTestFromBackup"),
					resource.TestCheckResourceAttr(s.ResourceName, "domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr(s.ResourceName, "node_count", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.db_version", "12.1.0.2"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.admin_password", "BEstrO0ng_#11"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_name", "dbback"),
					resource.TestCheckResourceAttr(s.ResourceName, "state", string(database.DatabaseLifecycleStateAvailable)),
				),
			},
		},
	})
}

// TestAccResourceDatabaseDBSystem_basic tests creation of a DBSystem with the minimum required properties
// to assert expected default values are set
func (s *ResourceDatabaseDBSystemTestSuite) TestAccResourceDatabaseDBSystem_basic() {
	// This test is a subset of TestAccResourceDatabaseDBSystem_allXX. It tests omitting optional params.
	if strings.Contains(getEnvSettingWithBlankDefault("suppressed_tests"), "DBSystem_basic") {
		s.T().Skip("Skipping subset dbsystem test.")
	}

	resource.Test(s.T(), resource.TestCase{
		Providers: s.Providers,
		Steps: []resource.TestStep{
			// verify create
			{
				Config: s.Config + `
				resource "oci_database_db_system" "t" {
					availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
					compartment_id = "${var.compartment_id}"
					subnet_id = "${oci_core_subnet.t.id}"
					database_edition = "ENTERPRISE_EDITION"
					disk_redundancy = "NORMAL"
					shape = "BM.DenseIO2.52"
					cpu_core_count = "2"
					ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
					domain = "${oci_core_subnet.t.dns_label}.${oci_core_virtual_network.t.dns_label}.oraclevcn.com"
					hostname = "myOracleDB"
					data_storage_size_in_gb = "256"
					license_model = "LICENSE_INCLUDED"
					node_count = "1"
					#fault_domains = ["FAULT-DOMAIN-1"]
					db_home {
						db_version = "12.1.0.2"
						database {
							"admin_password" = "BEstrO0ng_#11"
							"db_name" = "aTFdb"
						}
					}
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// DB System Resource tests
					resource.TestCheckResourceAttrSet(s.ResourceName, "id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "availability_domain"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "compartment_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "subnet_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "time_created"),
					resource.TestCheckResourceAttr(s.ResourceName, "database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr(s.ResourceName, "disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr(s.ResourceName, "shape", "BM.DenseIO2.52"),
					resource.TestCheckResourceAttr(s.ResourceName, "cpu_core_count", "2"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestMatchResourceAttr(s.ResourceName, "display_name", regexp.MustCompile(`dbsystem\d+`)),
					resource.TestCheckResourceAttr(s.ResourceName, "domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr(s.ResourceName, "node_count", "1"),
					//resource.TestCheckResourceAttr(s.ResourceName, "fault_domains.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.db_version", "12.1.0.2"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.admin_password", "BEstrO0ng_#11"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttr(s.ResourceName, "state", string(database.DatabaseLifecycleStateAvailable)),
				),
			},
		},
	})
}

// TestAccResourceDatabaseDBSystem_allBM tests DBsystems using Bare Metal instances.
func (s *ResourceDatabaseDBSystemTestSuite) TestAccResourceDatabaseDBSystem_allBM() {
	if strings.Contains(getEnvSettingWithBlankDefault("suppressed_tests"), "DBSystem_allBM") {
		s.T().Skip("Skipping BM test due to tenancy limits.")
	}

	resource.Test(s.T(), resource.TestCase{
		Providers: s.Providers,
		Steps: []resource.TestStep{
			// verify create
			{
				Config: s.Config + s.TokenFn(`
				resource "oci_database_db_system" "t" {
					availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
					compartment_id = "${var.compartment_id}"
					subnet_id = "${oci_core_subnet.t.id}"
					//backup_subnet_id = "${oci_core_subnet.t2.id}" // this requires a specific shape
					database_edition = "ENTERPRISE_EDITION"
					disk_redundancy = "NORMAL"
					shape = "BM.DenseIO2.52"
					cpu_core_count = "2"
					ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
					display_name = "{{.token}}"
					domain = "${oci_core_subnet.t.dns_label}.${oci_core_virtual_network.t.dns_label}.oraclevcn.com"
					hostname = "myOracleDB" // this will be lowercased server side
					data_storage_size_in_gb = "256"
					license_model = "LICENSE_INCLUDED"
					node_count = "1"
					#fault_domains = ["FAULT-DOMAIN-1"]
					db_home {
						db_version = "12.1.0.2"
						display_name = "-tf-db-home"
						database {
							"admin_password" = "BEstrO0ng_#11"
							"db_name" = "aTFdb"
							character_set = "AL32UTF8"
							ncharacter_set = "AL16UTF16"
							db_workload = "OLTP"
							pdb_name = "pdbName"
							db_backup_config {
								auto_backup_enabled = true
							}
						}
					}
				}
				data "oci_database_db_systems" "t" {
					compartment_id = "${var.compartment_id}"
					filter {
						name   = "display_name"
						values = ["${oci_database_db_system.t.display_name}"]
					}
				}
				data "oci_database_db_homes" "t" {
					compartment_id = "${var.compartment_id}"
					db_system_id = "${oci_database_db_system.t.id}"
					filter {
						name   = "db_system_id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_home" "t" {
					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.db_home_id}"
				}
				data "oci_database_databases" "t" {
					compartment_id = "${var.compartment_id}"
					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.id}"
					filter {
						name   = "db_name"
						values = ["${oci_database_db_system.t.db_home.0.database.0.db_name}"]
					}
				}
				data "oci_database_database" "t" {
					  database_id = "${data.oci_database_databases.t.databases.0.id}"
				}
				data "oci_database_db_nodes" "t" {
					compartment_id = "${var.compartment_id}"
					db_system_id = "${oci_database_db_system.t.id}"
					filter {
						name   = "db_system_id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_node" "t" {
					db_node_id = "${data.oci_database_db_nodes.t.db_nodes.0.id}"
				}`, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					// DB System Resource tests
					resource.TestCheckResourceAttrSet(s.ResourceName, "id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "availability_domain"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "compartment_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "subnet_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "time_created"),
					resource.TestCheckResourceAttr(s.ResourceName, "database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr(s.ResourceName, "disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr(s.ResourceName, "shape", "BM.DenseIO2.52"),
					resource.TestCheckResourceAttr(s.ResourceName, "cpu_core_count", "2"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestCheckResourceAttr(s.ResourceName, "display_name", s.Token),
					resource.TestCheckResourceAttr(s.ResourceName, "domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr(s.ResourceName, "node_count", "1"),
					// resource.TestCheckResourceAttr(s.ResourceName, "fault_domains.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.db_version", "12.1.0.2"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.display_name", "-tf-db-home"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.admin_password", "BEstrO0ng_#11"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.character_set", "AL32UTF8"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_workload", "OLTP"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.pdb_name", "pdbName"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_backup_config.0.auto_backup_enabled", "true"),
					resource.TestCheckResourceAttr(s.ResourceName, "state", string(database.DbSystemLifecycleStateAvailable)),

					// Data Source tests
					// DBSystems Data Source
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.#"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.#", "1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.availability_domain"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.compartment_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.subnet_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.time_created"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.shape", "BM.DenseIO2.52"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.cpu_core_count", "2"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.display_name", s.Token),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.license_model", "LICENSE_INCLUDED"),

					// The following fields are null when retrieved via data source. Some were never populated, some nulls might be BM vs VM behavior.
					//   maybe LIST summary vs GET behavior
					//	"backupSubnetId":null,
					//	"clusterName":null,
					//	"dataStorageSizeInGBs":null,
					//	"lastPatchHistoryEntryId":null,
					//	"lifecycleDetails":null,
					//	"nodeCount":null,
					//	"recoStorageSizeInGB":null,
					//	"scanDnsRecordId":null,
					//	"scanIpIds":null,
					//	"vipIds":null

					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.data_storage_size_in_gb"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.node_count"),
					//resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.fault_domains.#", "1"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.state", string(database.DbSystemLifecycleStateAvailable)),

					// DB Systems nested DB Home fields are not supported on the data source, so tests like below wont work if/until fetching the sub resource is implemented
					//resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.db_home"),
					//resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.db_home.0.db_version", "12.1.0.2"),

					// DBHomes
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.#"),
					resource.TestCheckResourceAttr("data.oci_database_db_homes.t", "db_homes.#", "1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.db_home_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.compartment_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.db_version"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.last_patch_history_entry_id"), // missing when null
					resource.TestCheckResourceAttr("data.oci_database_db_homes.t", "db_homes.0.state", string(database.DbHomeSummaryLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.db_system_id"),

					// DBHome
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "db_home_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "compartment_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "db_version"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "last_patch_history_entry_id"), // missing when null
					resource.TestCheckResourceAttr("data.oci_database_db_home.t", "state", string(database.DbHomeSummaryLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "db_system_id"),
					resource.TestCheckResourceAttr("data.oci_database_db_home.t", "display_name", "-tf-db-home"),

					// Databases
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.#"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.#", "1"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.character_set", "AL32UTF8"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.compartment_id"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.db_backup_config.0.auto_backup_enabled", "true"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.db_home_id"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.db_unique_name"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.db_workload", "OLTP"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.lifecycle_details"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.pdb_name", "pdbName"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.state", string(database.DatabaseLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.connection_strings.0.cdb_default"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.connection_strings.0.all_connection_strings.cdbDefault"),

					// Database
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "id"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "database_id"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "character_set", "AL32UTF8"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "compartment_id"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "db_backup_config.0.auto_backup_enabled", "true"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "db_home_id"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "db_name", "aTFdb"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "db_unique_name"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "db_workload", "OLTP"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_database.t", "lifecycle_details"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "pdb_name", "pdbName"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "state", string(database.DatabaseLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "connection_strings.0.cdb_default"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "connection_strings.0.all_connection_strings.cdbDefault"),

					// DB Nodes
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_system_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.#"),
					resource.TestCheckResourceAttr("data.oci_database_db_nodes.t", "db_nodes.#", "1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.db_node_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.db_system_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.hostname"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.state"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.fault_domain"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.vnic_id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.backup_vnic_id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.software_storage_size_in_gb"),

					// DB Node
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "db_node_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "db_system_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "hostname"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "state"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "fault_domain"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "vnic_id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "backup_vnic_id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "software_storage_size_in_gb"),
				),
			},
		},
	})
}

// TestAccResourceDatabaseDBSystem_allBM tests DBsystems using Virtual Machines.
func (s *ResourceDatabaseDBSystemTestSuite) TestAccResourceDatabaseDBSystem_allVM() {
	if strings.Contains(getEnvSettingWithBlankDefault("suppressed_tests"), "DBSystem_allVM") {
		s.T().Skip("Skipping VM test due to tenancy limits.")
	}

	var resId, resId2 string
	resource.Test(s.T(), resource.TestCase{
		Providers: s.Providers,
		Steps: []resource.TestStep{
			// verify create
			{
				Config: s.Config + s.TokenFn(`
				resource "oci_database_db_system" "t" {
					availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
					compartment_id = "${var.compartment_id}"
					subnet_id = "${oci_core_subnet.t.id}"
					//backup_subnet_id = "${oci_core_subnet.t2.id}" // this requires a specific shape
					database_edition = "ENTERPRISE_EDITION"
					disk_redundancy = "NORMAL"
					shape = "VM.Standard2.1"
					ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
					display_name = "{{.token}}"
					domain = "${oci_core_subnet.t.dns_label}.${oci_core_virtual_network.t.dns_label}.oraclevcn.com"
					hostname = "myOracleDB" // this will be lowercased server side
					data_storage_size_in_gb = "256"
					license_model = "LICENSE_INCLUDED"
					node_count = "1"
					#fault_domains = ["FAULT-DOMAIN-1"]
					db_home {
						db_version = "12.1.0.2"
						display_name = "-tf-db-home"
						database {
							"admin_password" = "BEstrO0ng_#11"
							"db_name" = "aTFdb"
							character_set = "AL32UTF8"
							//defined_tags = "${map("example-tag-namespace-all.example-tag", "originalValue")}" // service does not set tags of db tags in create
							//freeform_tags = {"Department" = "Finance"} // service does not set tags of db tags in create
							ncharacter_set = "AL16UTF16"
							db_workload = "OLTP"
							pdb_name = "pdbName"
							db_backup_config {
								auto_backup_enabled = true
							}
						}
					}
					defined_tags = "${map("example-tag-namespace-all.example-tag", "originalValue")}"
					freeform_tags = {"Department" = "Finance"}
				}
				data "oci_database_db_systems" "t" {
					compartment_id = "${var.compartment_id}"
					filter {
						name   = "id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_homes" "t" {
					compartment_id = "${var.compartment_id}"
					db_system_id = "${oci_database_db_system.t.id}"
					filter {
						name   = "db_system_id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_home" "t" {
					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.db_home_id}"
				}
				data "oci_database_databases" "t" {
					compartment_id = "${var.compartment_id}"
					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.id}"
					filter {
						name   = "db_name"
						values = ["${oci_database_db_system.t.db_home.0.database.0.db_name}"]
					}
				}
				data "oci_database_database" "t" {
					  database_id = "${data.oci_database_databases.t.databases.0.id}"
				}
				data "oci_database_db_nodes" "t" {
					compartment_id = "${var.compartment_id}"
					db_system_id = "${oci_database_db_system.t.id}"
					filter {
						name   = "db_system_id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_node" "t" {
					db_node_id = "${data.oci_database_db_nodes.t.db_nodes.0.id}"
				}`, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					// DB System Resource tests
					resource.TestCheckResourceAttrSet(s.ResourceName, "id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "availability_domain"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "compartment_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "subnet_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "time_created"),
					resource.TestCheckResourceAttr(s.ResourceName, "database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr(s.ResourceName, "disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr(s.ResourceName, "shape", "VM.Standard2.1"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "cpu_core_count"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestCheckResourceAttr(s.ResourceName, "display_name", s.Token),
					resource.TestCheckResourceAttr(s.ResourceName, "domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr(s.ResourceName, "license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_percentage", "80"),
					resource.TestCheckResourceAttr(s.ResourceName, "node_count", "1"),
					//resource.TestCheckResourceAttr(s.ResourceName, "fault_domains.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "reco_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "listener_port", "1521"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.db_version", "12.1.0.2"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.display_name", "-tf-db-home"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.admin_password", "BEstrO0ng_#11"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.character_set", "AL32UTF8"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_workload", "OLTP"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.pdb_name", "pdbName"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_backup_config.0.auto_backup_enabled", "true"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "state"),
					resource.TestCheckResourceAttr(s.ResourceName, "defined_tags.example-tag-namespace-all.example-tag", "originalValue"),
					resource.TestCheckResourceAttr(s.ResourceName, "freeform_tags.Department", "Finance"),

					// Data Source tests
					// DBSystems Data Source
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.#"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.#", "1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.availability_domain"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.compartment_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.subnet_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.time_created"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.shape", "VM.Standard2.1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.cpu_core_count"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.display_name", s.Token),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.data_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.data_storage_percentage", "80"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.node_count", "1"),
					//resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.fault_domains.#", "1"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.reco_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.listener_port", "1521"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.defined_tags.example-tag-namespace-all.example-tag", "originalValue"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.freeform_tags.Department", "Finance"),

					/* The following fields are null when retrieved via data source. Some were never populated, some nulls might be BM vs VM behavior.
					   maybe LIST summary vs GET behavior
						"backupSubnetId":null,
						"clusterName":null,
						"lastPatchHistoryEntryId":null,
						"lifecycleDetails":null,
						"scanDnsRecordId":null,
						"scanIpIds":null,
						"vipIds":null
					*/
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.data_storage_size_in_gb"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.node_count"),
					resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.state", string(database.DbSystemLifecycleStateAvailable)),

					// DB Systems nested DB Home fields are not supported on the data source, so tests like below wont work if/until fetching the sub resource is implemented
					//resource.TestCheckResourceAttrSet("data.oci_database_db_systems.t", "db_systems.0.db_home"),
					//resource.TestCheckResourceAttr("data.oci_database_db_systems.t", "db_systems.0.db_home.0.db_version", "12.1.0.2"),

					// DBHomes
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.#"),
					resource.TestCheckResourceAttr("data.oci_database_db_homes.t", "db_homes.#", "1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.db_home_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.compartment_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.db_version"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.last_patch_history_entry_id"), // missing when null
					resource.TestCheckResourceAttr("data.oci_database_db_homes.t", "db_homes.0.state", string(database.DbHomeSummaryLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_homes.t", "db_homes.0.db_system_id"),

					// DBHome
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "db_home_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "compartment_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "db_version"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "last_patch_history_entry_id"), // missing when null
					resource.TestCheckResourceAttr("data.oci_database_db_home.t", "state", string(database.DbHomeSummaryLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_home.t", "db_system_id"),
					resource.TestCheckResourceAttr("data.oci_database_db_home.t", "display_name", "-tf-db-home"),

					// Databases
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.#"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.#", "1"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.character_set", "AL32UTF8"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.compartment_id"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.db_backup_config.0.auto_backup_enabled", "true"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.db_home_id"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.db_unique_name"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.db_workload", "OLTP"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.lifecycle_details"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr("data.oci_database_databases.t", "databases.0.pdb_name", "pdbName"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.state"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.connection_strings.0.cdb_default"),
					resource.TestCheckResourceAttrSet("data.oci_database_databases.t", "databases.0.connection_strings.0.all_connection_strings.cdbDefault"),

					// Database
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "id"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "database_id"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "character_set", "AL32UTF8"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "compartment_id"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "db_backup_config.0.auto_backup_enabled", "true"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "db_home_id"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "db_name", "aTFdb"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "db_unique_name"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "db_workload", "OLTP"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "id"),
					//resource.TestCheckResourceAttrSet("data.oci_database_database.t", "lifecycle_details"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "pdb_name", "pdbName"),
					resource.TestCheckResourceAttr("data.oci_database_database.t", "state", string(database.DatabaseLifecycleStateAvailable)),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "connection_strings.0.cdb_default"),
					resource.TestCheckResourceAttrSet("data.oci_database_database.t", "connection_strings.0.all_connection_strings.cdbDefault"),

					// DB Nodes
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_system_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.#"),
					resource.TestCheckResourceAttr("data.oci_database_db_nodes.t", "db_nodes.#", "1"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.db_node_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.db_system_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.hostname"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.state"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.fault_domain"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.vnic_id"), // believe this is null when using FAKEHOSTSERIAL header
					//resource.TestCheckResourceAttrSet("data.oci_database_db_nodes.t", "db_nodes.0.backup_vnic_id"),
					resource.TestCheckResourceAttr("data.oci_database_db_nodes.t", "db_nodes.0.software_storage_size_in_gb", "200"),

					// DB Node
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "db_node_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "db_system_id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "hostname"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "id"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "state"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "time_created"),
					resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "fault_domain"),
					//resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "vnic_id"), // believe this is null when using FAKEHOSTSERIAL header
					//resource.TestCheckResourceAttrSet("data.oci_database_db_node.t", "backup_vnic_id"),
					resource.TestCheckResourceAttr("data.oci_database_db_node.t", "software_storage_size_in_gb", "200"),
					func(s *terraform.State) (err error) {
						resId, err = fromInstanceState(s, "oci_database_db_system.t", "id")
						return err
					},
				),
			},
			// verify update
			{
				Config: s.Config + s.TokenFn(`
				resource "oci_database_db_system" "t" {
					availability_domain = "${data.oci_identity_availability_domains.ADs.availability_domains.0.name}"
					compartment_id = "${var.compartment_id}"
					subnet_id = "${oci_core_subnet.t.id}"
					//backup_subnet_id = "${oci_core_subnet.t2.id}" // this requires a specific shape
					database_edition = "ENTERPRISE_EDITION"
					disk_redundancy = "NORMAL"
					cpu_core_count = "1"
					shape = "VM.Standard2.1"
					ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
					display_name = "{{.token}}"
					domain = "${oci_core_subnet.t.dns_label}.${oci_core_virtual_network.t.dns_label}.oraclevcn.com"
					hostname = "myOracleDB" // this will be lowercased server side
					data_storage_size_in_gb = "512"
					license_model = "LICENSE_INCLUDED"
					node_count = "1"
					#fault_domains = ["FAULT-DOMAIN-1"]
					db_home {
						db_version = "12.1.0.2"
						display_name = "-tf-db-home"
						database {
							"admin_password" = "BEstrO0ng_#11"
							"db_name" = "aTFdb"
							character_set = "AL32UTF8"
							defined_tags = "${map("example-tag-namespace-all.example-tag", "updateValue")}"
							freeform_tags = {"Department" = "Admin"}
							ncharacter_set = "AL16UTF16"
							db_workload = "OLTP"
							pdb_name = "pdbName"
							db_backup_config {
								auto_backup_enabled = false
							}
						}
					}
					defined_tags = "${map("example-tag-namespace-all.example-tag", "updateValue")}"
					freeform_tags = {"Department" = "Admin"}
				}
				data "oci_database_db_systems" "t" {
					compartment_id = "${var.compartment_id}"
					filter {
						name   = "id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_homes" "t" {
					compartment_id = "${var.compartment_id}"
					db_system_id = "${oci_database_db_system.t.id}"
					filter {
						name   = "db_system_id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_home" "t" {
					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.db_home_id}"
				}
				data "oci_database_databases" "t" {
					compartment_id = "${var.compartment_id}"
					db_home_id = "${data.oci_database_db_homes.t.db_homes.0.id}"
					filter {
						name   = "db_name"
						values = ["${oci_database_db_system.t.db_home.0.database.0.db_name}"]
					}
				}
				data "oci_database_database" "t" {
					  database_id = "${data.oci_database_databases.t.databases.0.id}"
				}
				data "oci_database_db_nodes" "t" {
					compartment_id = "${var.compartment_id}"
					db_system_id = "${oci_database_db_system.t.id}"
					filter {
						name   = "db_system_id"
						values = ["${oci_database_db_system.t.id}"]
					}
				}
				data "oci_database_db_node" "t" {
					db_node_id = "${data.oci_database_db_nodes.t.db_nodes.0.id}"
				}`, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					// DB System Resource tests
					resource.TestCheckResourceAttrSet(s.ResourceName, "id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "availability_domain"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "compartment_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "subnet_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "time_created"),
					resource.TestCheckResourceAttr(s.ResourceName, "database_edition", "ENTERPRISE_EDITION"),
					resource.TestCheckResourceAttr(s.ResourceName, "disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr(s.ResourceName, "shape", "VM.Standard2.1"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "cpu_core_count"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestCheckResourceAttr(s.ResourceName, "display_name", s.Token),
					resource.TestCheckResourceAttr(s.ResourceName, "domain", "tfsubnet.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr(s.ResourceName, "license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_size_in_gb", "512"),
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_percentage", "80"),
					resource.TestCheckResourceAttr(s.ResourceName, "node_count", "1"),
					//resource.TestCheckResourceAttr(s.ResourceName, "fault_domains.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "reco_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "listener_port", "1521"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.db_version", "12.1.0.2"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.display_name", "-tf-db-home"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.admin_password", "BEstrO0ng_#11"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.character_set", "AL32UTF8"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.defined_tags.example-tag-namespace-all.example-tag", "updateValue"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.freeform_tags.Department", "Admin"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.ncharacter_set", "AL16UTF16"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_workload", "OLTP"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.pdb_name", "pdbName"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_backup_config.0.auto_backup_enabled", "false"),
					resource.TestCheckResourceAttr(s.ResourceName, "state", string(database.DbSystemLifecycleStateAvailable)),
					resource.TestCheckResourceAttr(s.ResourceName, "defined_tags.example-tag-namespace-all.example-tag", "updateValue"),
					resource.TestCheckResourceAttr(s.ResourceName, "freeform_tags.Department", "Admin"),

					func(s *terraform.State) (err error) {
						resId2, err = fromInstanceState(s, "oci_database_db_system.t", "id")
						if resId != resId2 {
							return fmt.Errorf("expected same ocids, got different")
						}
						return err
					},
				),
			},
		},
	})
}

// TestAccResourceDatabaseDBSystem_Exadata tests DBsystems using Exadata
func (s *ResourceDatabaseDBSystemTestSuite) TestAccResourceDatabaseDBSystem_Exadata() {

	if strings.Contains(getEnvSettingWithBlankDefault("suppressed_tests"), "DBSystem_Exadata") {
		s.T().Skip("Skipping suppressed DBSystem_Exadata")
	}

	resource.Test(s.T(), resource.TestCase{
		Providers: s.Providers,
		Steps: []resource.TestStep{
			// verify create
			{
				Config: s.Config + `
				data "oci_identity_availability_domain" "ad" {
  					compartment_id 		= "${var.compartment_id}"
  					ad_number      		= 3
				}
				resource "oci_core_subnet" "exadata_subnet" {
					availability_domain = "${data.oci_identity_availability_domain.ad.name}"
					cidr_block          = "10.1.22.0/24"
					display_name        = "ExadataSubnet"
					compartment_id      = "${var.compartment_id}"
					vcn_id              = "${oci_core_virtual_network.t.id}"
					route_table_id      = "${oci_core_virtual_network.t.default_route_table_id}"
					dhcp_options_id     = "${oci_core_virtual_network.t.default_dhcp_options_id}"
					security_list_ids   = ["${oci_core_virtual_network.t.default_security_list_id}", "${oci_core_security_list.exadata_shapes_security_list.id}"]
					dns_label           = "subnetexadata1"
				}
				resource "oci_core_subnet" "exadata_backup_subnet" {
					availability_domain = "${data.oci_identity_availability_domain.ad.name}"
					cidr_block          = "10.1.23.0/24"
					display_name        = "ExadataBackupSubnet"
					compartment_id      = "${var.compartment_id}"
					vcn_id              = "${oci_core_virtual_network.t.id}"
					route_table_id      = "${oci_core_virtual_network.t.default_route_table_id}"
					dhcp_options_id     = "${oci_core_virtual_network.t.default_dhcp_options_id}"
					security_list_ids   = ["${oci_core_virtual_network.t.default_security_list_id}"]
					dns_label           = "subnetexadata2"
				}
				
				resource "oci_core_security_list" "exadata_shapes_security_list" {
					compartment_id = "${var.compartment_id}"
					vcn_id         = "${oci_core_virtual_network.t.id}"
					display_name   = "ExadataSecurityList"

					ingress_security_rules {
						source    = "10.1.22.0/24"
						protocol  = "6"
					}
			
					ingress_security_rules {
						source    = "10.1.22.0/24"
						protocol  = "1"
					}

					egress_security_rules {
						destination = "10.1.22.0/24"
						protocol    = "6"
					}
			
					egress_security_rules {
						destination = "10.1.22.0/24"
						protocol    = "1"
					}
				}
			
				resource "oci_database_db_system" "t" {
					availability_domain = "${data.oci_identity_availability_domain.ad.name}"
					compartment_id = "${var.compartment_id}"
					subnet_id = "${oci_core_subnet.exadata_subnet.id}"
					backup_subnet_id = "${oci_core_subnet.exadata_backup_subnet.id}"
					database_edition = "ENTERPRISE_EDITION_EXTREME_PERFORMANCE"
					disk_redundancy = "NORMAL"
					shape = "Exadata.Quarter1.84"
					cpu_core_count = "22"
					ssh_public_keys = ["ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"]
					domain = "${oci_core_subnet.exadata_subnet.dns_label}.${oci_core_virtual_network.t.dns_label}.oraclevcn.com"
					hostname = "myOracleDB"
					data_storage_size_in_gb = "256"
					license_model = "LICENSE_INCLUDED"
					node_count = "1"
					time_zone = "US/Pacific"
					db_home {
						db_version = "12.1.0.2"
						database {
							"admin_password" = "BEstrO0ng_#11"
							"db_name" = "aTFdb"
						}
					}
				}`,

				Check: resource.ComposeAggregateTestCheckFunc(
					// DB System Resource tests
					resource.TestCheckResourceAttrSet(s.ResourceName, "id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "availability_domain"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "compartment_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "subnet_id"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "time_created"),
					resource.TestCheckResourceAttr(s.ResourceName, "database_edition", "ENTERPRISE_EDITION_EXTREME_PERFORMANCE"),
					resource.TestCheckResourceAttr(s.ResourceName, "disk_redundancy", "NORMAL"),
					resource.TestCheckResourceAttr(s.ResourceName, "shape", "Exadata.Quarter1.84"),
					resource.TestCheckResourceAttr(s.ResourceName, "cpu_core_count", "22"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.#", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "ssh_public_keys.0", "ssh-rsa KKKLK3NzaC1yc2EAAAADAQABAAABAQC+UC9MFNA55NIVtKPIBCNw7++ACXhD0hx+Zyj25JfHykjz/QU3Q5FAU3DxDbVXyubgXfb/GJnrKRY8O4QDdvnZZRvQFFEOaApThAmCAM5MuFUIHdFvlqP+0W+ZQnmtDhwVe2NCfcmOrMuaPEgOKO3DOW6I/qOOdO691Xe2S9NgT9HhN0ZfFtEODVgvYulgXuCCXsJs+NUqcHAOxxFUmwkbPvYi0P0e2DT8JKeiOOC8VKUEgvVx+GKmqasm+Y6zHFW7vv3g2GstE1aRs3mttHRoC/JPM86PRyIxeWXEMzyG5wHqUu4XZpDbnWNxi6ugxnAGiL3CrIFdCgRNgHz5qS1l MustWin"),
					resource.TestMatchResourceAttr(s.ResourceName, "display_name", regexp.MustCompile(`dbsystem\d+`)),
					resource.TestCheckResourceAttr(s.ResourceName, "domain", "subnetexadata1.tfvcn.oraclevcn.com"),
					resource.TestCheckResourceAttrSet(s.ResourceName, "hostname"), // see comment in SetData fn as to why this is removed
					resource.TestCheckResourceAttr(s.ResourceName, "data_storage_size_in_gb", "256"),
					resource.TestCheckResourceAttr(s.ResourceName, "license_model", "LICENSE_INCLUDED"),
					resource.TestCheckResourceAttr(s.ResourceName, "node_count", "1"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.db_version", "12.1.0.2"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.admin_password", "BEstrO0ng_#11"),
					resource.TestCheckResourceAttr(s.ResourceName, "db_home.0.database.0.db_name", "aTFdb"),
					resource.TestCheckResourceAttr(s.ResourceName, "state", string(database.DatabaseLifecycleStateAvailable)),
					resource.TestCheckResourceAttr(s.ResourceName, "time_zone", "US/Pacific"),
				),
			},
		},
	})
}

func TestResourceDatabaseDBSystemTestSuite(t *testing.T) {
	suite.Run(t, new(ResourceDatabaseDBSystemTestSuite))
}
