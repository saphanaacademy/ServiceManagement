package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

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

func (c *ServiceManagementPlugin) Run(cliConnection plugin.CliConnection, args []string) {

	// flags
	flags := flag.NewFlagSet("service-manager-service-instances", flag.ExitOnError)
	serviceOfferingName := flags.String("offering", "hana", "Service offering")
	servicePlanName := flags.String("plan", "hdi-shared", "Service plan")
	showCredentials := flags.Bool("credentials", false, "Show credentials")
	outputFormat := flags.String("o", "Txt", "Show as JSON | SQLTools | Txt)")
	err := flags.Parse(args[2:])
	handleError(err)

	if args[0] == "service-manager-service-instances" {
		if len(args) < 2 {
			fmt.Println("Please specify an instance of service manager")
			return
		}
		serviceManagerName := args[1]

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
		serviceKeyName := "sk-" + args[0]
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
		q2.Add("fieldQuery", "catalog_name eq '"+*serviceOfferingName+"'")
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
			fmt.Printf("Service offering not found: %s\n", *serviceOfferingName)
		} else {
			// get id of service plan for offering
			serviceOfferingId, err := jsonparser.GetString(body2Bytes, "items", "[0]", "id")
			url3, err := jsonparser.GetString([]byte(strings.Join(serviceKey, "")), "sm_url")
			handleError(err)
			req3, err := http.NewRequest("GET", url3+"/v1/service_plans", nil)
			handleError(err)
			q3 := req3.URL.Query()
			q3.Add("fieldQuery", "catalog_name eq '"+*servicePlanName+"' and service_offering_id eq '"+serviceOfferingId+"'")
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
				fmt.Printf("Service plan not found: %s\n", *servicePlanName)
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

				switch outputFormat {
				case "json":
					fmt.Printf(`{"service_offering": "%s", "service_plan": "%s", "num_items": %d, "items": [`, *serviceOfferingName, *servicePlanName, numItems)
				case "sqltools":
					fmt.Printf(`"sqltools.connections": [`)
				case "txt":
					fmt.Printf("%d items found for service offering %s and service plan %s.\n", numItems, *serviceOfferingName, *servicePlanName)
				}

				// for each item
				var item = 0
				jsonparser.ArrayEach(body4Bytes, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					item = item + 1
					id, _ := jsonparser.GetString(value, "id")
					name, _ := jsonparser.GetString(value, "name")
					createdAt, _ := jsonparser.GetString(value, "created_at")
					updatedAt, _ := jsonparser.GetString(value, "updated_at")
					ready, _ := jsonparser.GetBoolean(value, "ready")
					usable, _ := jsonparser.GetBoolean(value, "usable")

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
					hdiuser, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "hdi_user")
					hdipassword, _ := jsonparser.GetString(body5Bytes, "items", "[0]", "credentials", "hdi_password")

					if outputFormat == "json" {
						if item > 1 {
							fmt.Printf(`,`)
						}
						fmt.Printf(`{"name": "%s", "id": "%s", "created_at": "%s", "updated_at": "%s", "ready": %t, "usable": %t, "schema": "%s", "host": "%s", "port": "%s", "url": "%s", "driver": "%s"`, name, id, createdAt, updatedAt, ready, usable, schema, host, port, url, driver)
						if *showCredentials {
							fmt.Printf(`, "user": "%s", "password": "%s", "hdi_user": "%s", "hdi_password": "%s", "certificate": "%s"`, user, password, hdiuser, hdipassword, certificate)
						}
						fmt.Printf(`}`)
					} else if outputFormat == "sqltools" {
						if item > 1 {
							fmt.Printf(`,`)
						}
						fmt.Printf(`{"name": "%s", "dialect": "SAPHana", "server": "%s", "port": %s, "database": "%s", "username": "%s", "password": "%s", "connectionTimeout": 30, "hanaOptions": {"encrypt": true, "sslValidateCertificate": true, "sslCryptoProvider": "openssl", "sslTrustStore": "%s"}},`, serviceManagerName+":"+name, host, port, schema, user, password, certificate)
						fmt.Printf(`{"name": "%s", "dialect": "SAPHana", "server": "%s", "port": %s, "database": "%s", "username": "%s", "password": "%s", "connectionTimeout": 30, "hanaOptions": {"encrypt": true, "sslValidateCertificate": true, "sslCryptoProvider": "openssl", "sslTrustStore": "%s"}}`, serviceManagerName+":"+name+":OWNER", host, port, schema, hdiuser, hdipassword, certificate)
					} else {
						//txt
						fmt.Printf("\nName: %s \nId: %s \nCreatedAt: %s \nUpdatedAt: %s \nReady: %t \nUsable: %t \nSchema: %s \nHost: %s \nPort: %s \nURL: %s \nDriver: %s\n", name, id, createdAt, updatedAt, ready, usable, schema, host, port, url, driver)
						if *showCredentials {
							fmt.Printf("User: %s \nPassword: %s \nHDIUser: %s \nHDIPassword: %s \nCertificate: %s \n", user, password, hdiuser, hdipassword, certificate)
						}
					}

				}, "items")

				switch outputFormat {
				case "json":
					fmt.Println(`]}`)
				case "sqltools":
					fmt.Println(`],`)
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
			Build: 0,
		},
		MinCliVersion: plugin.VersionType{
			Major: 6,
			Minor: 7,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "service-manager-service-instances",
				Alias:    "smsi",
				HelpText: "Show service manager service instances for a service offering and plan.",
				UsageDetails: plugin.Usage{
					Usage: "cf service-manager-service-instances <SERVICE_MANAGER_INSTANCE> [-offering <SERVICE_OFFERING>] [-plan <SERVICE_PLAN>] [-credentials] [-o JSON | SQLTools | Txt]",
					Options: map[string]string{
						"credentials": "Show credentials",
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
