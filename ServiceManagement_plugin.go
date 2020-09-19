package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	plugin_models "code.cloudfoundry.org/cli/plugin/models"

	"code.cloudfoundry.org/cli/plugin"

	"github.com/buger/jsonparser"
)

type ServiceManagementPlugin struct {
	serviceOfferingName *string
	servicePlanName     *string
	showCredentials     *bool
	outputFormat        *string
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

type Containers struct {
	ContainerID string
	TenantID    string
}

func (c *ServiceManagementPlugin) Run(cliConnection plugin.CliConnection, args []string) {

	// flags
	flags := flag.NewFlagSet("service-manager-service-instances", flag.ExitOnError)
	serviceOfferingName := flags.String("offering", "hana", "Service offering")
	servicePlanName := flags.String("plan", "hdi-shared", "Service plan")
	showCredentials := flags.Bool("credentials", false, "Show credentials")
	includeMeta := flags.Bool("meta", false, "Include Meta containers")
	includeOwner := flags.Bool("owner", false, "Include Owner credentials")
	outputFormat := flags.String("o", "Txt", "Show as JSON | SQLTools | Txt)")
	err := flags.Parse(args[1:])
	handleError(err)

	serviceNameGiven := false

	if args[0] == "service-manager-service-instances" {
		/*
			if len(args) < 2 {
				fmt.Println("Please specify an instance of service manager")
				return
			}
		*/

		// https://github.com/cloudfoundry/cli/tree/master/plugin/plugin_examples
		// https://github.com/cloudfoundry/cli/blob/master/plugin/plugin_examples/DOC.md

		// org := plugin_models.Organization{}
		// org, err = cliConnection.GetCurrentOrg()
		// handleError(err)
		// fmt.Println("org = " + org.OrganizationFields.Name)

		serviceManagerName := "Unknown"
		//fmt.Println("args[0] = " + args[0])
		//fmt.Println("args[1] = " + args[1])

		if len(args) > 1 {
			if args[1][0] == '-' {
				//fmt.Println("no sm in args")
				err = flags.Parse(args[1:])
				handleError(err)
			} else {
				serviceNameGiven = true
				serviceManagerName = args[1]
				err = flags.Parse(args[2:])
				handleError(err)
			}
		}

		// return

		if !serviceNameGiven {

			svcs := []plugin_models.GetServices_Model{}

			svcs, err = cliConnection.GetServices()
			handleError(err)

			foundSvcs := []plugin_models.GetServices_Model{}

			for i := 0; i < len(svcs); i++ {
				//fmt.Println("Service Name: " + svcs[i].Name)
				if svcs[i].Service.Name == "service-manager" {
					//fmt.Println("Service Type: " + svcs[i].Service.Name)
					if svcs[i].ServicePlan.Name == "container" {
						//fmt.Println("Service Plan: " + svcs[i].ServicePlan.Name)
						foundSvcs = append(foundSvcs, svcs[i])
					}
				}
			}

			if len(foundSvcs) >= 1 {
				if len(foundSvcs) == 1 {
					serviceManagerName = foundSvcs[0].Name
				} else {
					for i := 0; i < len(foundSvcs); i++ {
						fmt.Println(fmt.Sprintf("%d :", i) + foundSvcs[i].Name)
					}
					fmt.Print("Which service-manager?: ")
					var input string
					fmt.Scanln(&input)
					//fmt.Print(input)
					smidx, _ := strconv.Atoi(input)
					serviceManagerName = foundSvcs[smidx].Name
				}
			} else {
				fmt.Println("Please create at least one instance of service-manager with plan type container.")
				return
			}
		}

		fmt.Println("service manager = " + serviceManagerName)

		serviceOfferingName := strings.ToLower(*serviceOfferingName)
		servicePlanName := strings.ToLower(*servicePlanName)

		// validate output format
		outputFormat := strings.ToLower(*outputFormat)
		switch outputFormat {
		case "json", "sqltools", "txt":
		default:
			fmt.Println("Output format must be JSON, SQLTools or Txt")
			return
		}

		// check instance exists
		_, err := cliConnection.GetService(serviceManagerName)
		handleError(err)

		// create service key
		serviceKeyName := "sk-" + serviceManagerName
		_, err = cliConnection.CliCommandWithoutTerminalOutput("create-service-key", serviceManagerName, serviceKeyName)
		handleError(err)

		// get service key
		serviceKey, err := cliConnection.CliCommandWithoutTerminalOutput("service-key", serviceManagerName, serviceKeyName)
		handleError(err)

		// cleanup headers to make parsable as JSON
		serviceKey[0] = ""
		serviceKey[1] = ""

		// authenticate to service manager REST API
		cli := &http.Client{}
		url1, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "url")
		handleError(err)
		req1, err := http.NewRequest("POST", url1+"/oauth/token?grant_type=client_credentials", nil)
		handleError(err)
		req1.Header.Set("Content-Type", "application/json")
		clientid, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "clientid")
		handleError(err)
		clientsecret, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "clientsecret")
		handleError(err)
		req1.SetBasicAuth(clientid, clientsecret)
		res1, err := cli.Do(req1)
		handleError(err)
		defer res1.Body.Close()
		body1Bytes, err := ioutil.ReadAll(res1.Body)
		handleError(err)
		accessToken, err := jsonparser.GetString(body1Bytes, "access_token")

		// get id of service offering
		url2, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "sm_url")
		handleError(err)
		req2, err := http.NewRequest("GET", url2+"/v1/service_offerings", nil)
		handleError(err)
		q2 := req2.URL.Query()
		q2.Add("fieldQuery", "catalog_name eq '"+serviceOfferingName+"'")
		req2.URL.RawQuery = q2.Encode()
		req2.Header.Set("Authorization", "Bearer "+accessToken)
		res2, err := cli.Do(req2)
		handleError(err)
		defer res2.Body.Close()
		body2Bytes, err := ioutil.ReadAll(res2.Body)
		handleError(err)
		numItems, err := jsonparser.GetInt(body2Bytes, "num_items")
		handleError(err)
		if numItems < 1 {
			fmt.Printf("Service offering not found: %s\n", serviceOfferingName)
		} else {
			// get id of service plan for offering
			serviceOfferingId, err := jsonparser.GetString(body2Bytes, "items", "[0]", "id")
			url3, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "sm_url")
			handleError(err)
			req3, err := http.NewRequest("GET", url3+"/v1/service_plans", nil)
			handleError(err)
			q3 := req3.URL.Query()
			q3.Add("fieldQuery", "catalog_name eq '"+servicePlanName+"' and service_offering_id eq '"+serviceOfferingId+"'")
			req3.URL.RawQuery = q3.Encode()
			req3.Header.Set("Authorization", "Bearer "+accessToken)
			res3, err := cli.Do(req3)
			handleError(err)
			defer res3.Body.Close()
			body3Bytes, err := ioutil.ReadAll(res3.Body)
			handleError(err)
			numItems, err = jsonparser.GetInt(body3Bytes, "num_items")
			handleError(err)
			if numItems < 1 {
				fmt.Printf("Service plan not found: %s\n", servicePlanName)
			} else {
				servicePlanId, err := jsonparser.GetString(body3Bytes, "items", "[0]", "id")

				// get service instances for service plan
				url4, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "sm_url")
				handleError(err)
				req4, err := http.NewRequest("GET", url4+"/v1/service_instances", nil)
				handleError(err)
				q4 := req4.URL.Query()
				q4.Add("fieldQuery", "service_plan_id eq '"+servicePlanId+"'")
				req4.URL.RawQuery = q4.Encode()
				req4.Header.Set("Authorization", "Bearer "+accessToken)
				res4, err := cli.Do(req4)
				handleError(err)
				defer res4.Body.Close()
				body4Bytes, err := ioutil.ReadAll(res4.Body)
				handleError(err)
				numItems, err = jsonparser.GetInt(body4Bytes, "num_items")
				handleError(err)

				foundContainers := []Containers{}

				// for each item
				var item = 0
				var isMeta = false
				jsonparser.ArrayEach(body4Bytes, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					isMeta = false
					id, _ := jsonparser.GetString(value, "id")

					// get service binding
					url5, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "sm_url")
					handleError(err)
					req5, err := http.NewRequest("GET", url5+"/v1/service_bindings", nil)
					handleError(err)
					q5 := req5.URL.Query()
					q5.Add("fieldQuery", "service_instance_id eq '"+id+"'")
					req5.URL.RawQuery = q5.Encode()
					req5.Header.Set("Authorization", "Bearer "+accessToken)
					res5, err := cli.Do(req5)
					handleError(err)
					defer res5.Body.Close()
					body5Bytes, err := ioutil.ReadAll(res5.Body)
					handleError(err)

					tenantID, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "labels", "tenant_id", "[0]")
					var splits = strings.Split(tenantID, "-")
					if splits[0] == "TENANT" {
						isMeta = true
					}

					if !isMeta || (isMeta && *includeMeta) {
						//fmt.Printf("%d: %s \n", item, tenantID)
						container := Containers{ContainerID: id, TenantID: tenantID}
						foundContainers = append(foundContainers, container)
						item = item + 1
					}
				}, "items")

				whichID := "ALL"

				if len(foundContainers) > 1 {
					fmt.Printf("%d: %s \n", 0, "Include All")
					for i := 0; i < len(foundContainers); i++ {
						fmt.Printf("%d: %s \n", i+1, foundContainers[i].TenantID)
					}

					fmt.Print("Which container?: ")
					var input string
					fmt.Scanln(&input)
					cidx, _ := strconv.Atoi(input)
					if cidx == 0 {
						fmt.Printf("Using: %s \n", "All Containers")
					} else {
						whichContainer := foundContainers[cidx-1].TenantID
						fmt.Printf("Using: %s \n", whichContainer)
						whichID = foundContainers[cidx-1].ContainerID
					}
				} else {
					whichID = foundContainers[0].ContainerID
				}

				switch outputFormat {
				case "json":
					fmt.Printf(`{"service_offering": "%s", "service_plan": "%s", "num_items": %d, "items": [`, serviceOfferingName, servicePlanName, numItems)
				case "sqltools":
					fmt.Printf(`{"sqltools.connections": [`)
				case "txt":
					fmt.Printf("%d items found for service offering %s and service plan %s.\n", numItems, serviceOfferingName, servicePlanName)
				}

				// for each item
				item = 0
				isMeta = false
				jsonparser.ArrayEach(body4Bytes, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					//item = item + 1
					isMeta = false
					id, _ := jsonparser.GetString(value, "id")
					name, _ := jsonparser.GetString(value, "name")
					createdAt, _ := jsonparser.GetString(value, "created_at")
					updatedAt, _ := jsonparser.GetString(value, "updated_at")
					ready, _ := jsonparser.GetBoolean(value, "ready")
					usable, _ := jsonparser.GetBoolean(value, "usable")

					if (whichID == id) || (whichID == "ALL") {

						// get service binding
						url5, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "sm_url")
						handleError(err)
						req5, err := http.NewRequest("GET", url5+"/v1/service_bindings", nil)
						handleError(err)
						q5 := req5.URL.Query()
						q5.Add("fieldQuery", "service_instance_id eq '"+id+"'")
						req5.URL.RawQuery = q5.Encode()
						req5.Header.Set("Authorization", "Bearer "+accessToken)
						res5, err := cli.Do(req5)
						handleError(err)
						defer res5.Body.Close()
						body5Bytes, err := ioutil.ReadAll(res5.Body)
						handleError(err)
						host, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "host")
						port, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "port")
						driver, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "driver")
						schema, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "schema")
						certificate, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "certificate")
						re := regexp.MustCompile(`\n`)
						certificate = re.ReplaceAllString(certificate, "")
						url, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "url")
						user, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "user")
						password, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "password")
						var hdiuser = ""
						var hdipassword = ""
						if servicePlanName == "hdi-shared" {
							hdiuser, _ = jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "hdi_user")
							hdipassword, _ = jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "hdi_password")
						}
						tenantID, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "labels", "tenant_id", "[0]")
						var splits = strings.Split(tenantID, "-")
						if splits[0] == "TENANT" {
							isMeta = true
						}

						//name = serviceManagerName + ":" + tenantID
						//name = tenantID

						// Need to use the SAPCP API to get the subdomain from the subaccount GUID which is the tenantID
						// sapcp get accounts/subaccount b44f32d4-6e31-4d95-b17f-6c6fcdb37e1f

						if !isMeta || (isMeta && *includeMeta) {
							item = item + 1
							if outputFormat == "json" {
								if item > 1 {
									fmt.Printf(`,`)
								}
								fmt.Printf(`{"name": "%s", "id": "%s", "created_at": "%s", "updated_at": "%s", "ready": %t, "usable": %t, "schema": "%s", "host": "%s", "port": "%s", "url": "%s", "driver": "%s"`, name, id, createdAt, updatedAt, ready, usable, schema, host, port, url, driver)
								if *showCredentials {
									fmt.Printf(`, "user": "%s", "password": "%s", "certificate": "%s"`, user, password, certificate)
									if servicePlanName == "hdi-shared" && *includeOwner {
										fmt.Printf(`, "hdi_user": "%s", "hdi_password": "%s"`, hdiuser, hdipassword)
									}
								}
								fmt.Printf(`}`)
							} else if outputFormat == "sqltools" {
								if item > 1 {
									fmt.Printf(`,`)
								}
								fmt.Printf(`{"name": "%s", "dialect": "SAPHana", "server": "%s", "port": %s, "database": "%s", "username": "%s", "password": "%s", "connectionTimeout": 30, "hanaOptions": {"encrypt": true, "sslValidateCertificate": true, "sslCryptoProvider": "openssl", "sslTrustStore": "%s"}}`, serviceManagerName+":"+tenantID, host, port, schema, user, password, certificate)
								if servicePlanName == "hdi-shared" && *includeOwner {
									fmt.Printf(`,{"name": "%s", "dialect": "SAPHana", "server": "%s", "port": %s, "database": "%s", "username": "%s", "password": "%s", "connectionTimeout": 30, "hanaOptions": {"encrypt": true, "sslValidateCertificate": true, "sslCryptoProvider": "openssl", "sslTrustStore": "%s"}}`, serviceManagerName+":"+tenantID+":OWNER", host, port, schema, hdiuser, hdipassword, certificate)
								}
							} else {
								//txt
								fmt.Printf("\nName: %s \nId: %s \nCreatedAt: %s \nUpdatedAt: %s \nReady: %t \nUsable: %t \nSchema: %s \nHost: %s \nPort: %s \nURL: %s \nDriver: %s\n", name, id, createdAt, updatedAt, ready, usable, schema, host, port, url, driver)
								if *showCredentials {
									fmt.Printf("User: %s \nPassword: %s \nCertificate: %s \n", user, password, certificate)
									if servicePlanName == "hdi-shared" && *includeOwner {
										fmt.Printf("HDIUser: %s \nHDIPassword: %s \n", hdiuser, hdipassword)
									}
								}
								fmt.Printf("TenantID: %s \n", tenantID)
							}
						}
					}
				}, "items")

				switch outputFormat {
				case "json":
					fmt.Println(`]}`)
				case "sqltools":
					fmt.Println(`]}`)
				}
			}

		}

		// delete service key
		_, err = cliConnection.CliCommandWithoutTerminalOutput("delete-service-key", serviceManagerName, serviceKeyName, "-f")
		handleError(err)
	}
}

func (c *ServiceManagementPlugin) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "ServiceManagement",
		Version: plugin.VersionType{
			Major: 1,
			Minor: 0,
			Build: 5,
		},
		MinCliVersion: plugin.VersionType{
			Major: 6,
			Minor: 7,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "service-manager-service-instances",
				Alias:    "sm",
				HelpText: "Show service manager service instances for a service offering and plan.",
				UsageDetails: plugin.Usage{
					Usage: "cf service-manager-service-instances [SERVICE_MANAGER_INSTANCE] [-offering <SERVICE_OFFERING>] [-plan <SERVICE_PLAN>] [--credentials] [--meta] [--owner] [-o JSON | SQLTools | Txt]",
					Options: map[string]string{
						"credentials": "Show credentials",
						"meta":        "Include Meta containers",
						"owner":       "Include Owner credentials",
						"o":           "Show as JSON | SQLTools | Txt (default 'Txt')",
						"offering":    "Service offering (default 'hana')",
						"plan":        "Service plan (default 'hdi-shared')",
					},
				},
			},
		},
	}
}

func main() {
	plugin.Start(new(ServiceManagementPlugin))
}
