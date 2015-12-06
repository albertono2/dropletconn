package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
)

type CommandConfig struct {
	forceUpdate   bool
	listPublicIps bool

	command string
}

func main() {
	var forceUpdate bool
	var listPublicIps bool

	flag.BoolVar(&forceUpdate, "force-update", false, "Force update the cache file")
	flag.BoolVar(&listPublicIps, "list-public-ip", false, "If the list command is used, only print out list of public IPs")
	flag.Parse()

	command := flag.Arg(0)

	nArgs := flag.NArg()
	if nArgs < 1 {
		fmt.Println("Not enough arguments")
		return
	}

	runningConf := CommandConfig{forceUpdate, listPublicIps, command}

	switch command {
	case "config":
		if err := createConfig(); err != nil {
			fmt.Printf("Error while creating config. Error: %s\n", err.Error())
		}
	case "c", "connect":
		connectToDroplet(runningConf)
	case "l", "list":
		filterExpressions := make([]string, 0)
		if nArgs > 1 {
			filterExpressions = flag.Args()[1:]
		}
		listDropletsInfo(filterExpressions, runningConf)
	case "completion":
		printCompletions(runningConf)
	default:
		fmt.Println("Unknown command")
	}
}

func connectToDroplet(runningConf CommandConfig) {
	nArgs := flag.NArg()
	if nArgs < 2 {
		fmt.Println("No droplet name given")
		return
	}

	inputDropletName := flag.Arg(1)
	var extraCmdOptions []string
	if nArgs > 2 {
		extraCmdOptions = flag.Args()[2:]
	}

	droplets, err := getDropletsFromApi(runningConf.forceUpdate)

	if err != nil {
		fmt.Printf("Unable to get droplets. Error: %s\n", err.Error())
		return
	} else if len(droplets) == 0 {
		fmt.Println("No droplets found in account")
		return
	}

	var matchedDropletInfo *DropletInfo
	inputDropletNameLower := strings.ToLower(inputDropletName)
	for _, dropletInfo := range droplets {
		dropletNameLower := strings.ToLower(dropletInfo.Name)
		if inputDropletNameLower == dropletNameLower {
			matchedDropletInfo = &dropletInfo
			break
		}
	}

	if matchedDropletInfo == nil {
		fmt.Printf("No match found for \"%s\"\n", inputDropletName)
		return
	}

	firstPublicIpAddress := ""
	for _, interfaceInfo := range matchedDropletInfo.Networks.V4 {
		if interfaceInfo.Type == "public" {
			firstPublicIpAddress = interfaceInfo.IpAddress
			break
		}

	}
	if firstPublicIpAddress == "" {
		fmt.Printf("No public IP address found for droplet \"%s\"\n", matchedDropletInfo.Name)
		return
	}

	config, err := getConfig()
	if err != nil {
		fmt.Printf("Unable to get config. Error: %s\n", err.Error())
		return
	}

	cmdOptions := []string{firstPublicIpAddress}
	if config.DefaultUser != "" {
		cmdOptions = append(cmdOptions, "-l")
		cmdOptions = append(cmdOptions, config.DefaultUser)
	}
	if config.DefaultKeyFileName != "" {
		keyFileName := config.DefaultKeyFileName
		keyFilePath, err := getAbsoluteFilePath(keyFileName)
		if err != nil {
			fmt.Printf("Unable to get absolute ssh key file path. Error: %s\n", err.Error())
			return
		}

		cmdOptions = append(cmdOptions, "-i")
		cmdOptions = append(cmdOptions, keyFilePath)
	}

	for _, extraOpt := range extraCmdOptions {
		cmdOptions = append(cmdOptions, extraOpt)
	}

	fmt.Printf("Connecting to \"%s\"\n", inputDropletName)
	cmd := exec.Command("ssh", cmdOptions...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		fmt.Printf("Error while trying to run ssh. Error: %s\n", err.Error())
	}
}

func listDropletsInfo(filterExpresions []string, runningConf CommandConfig) {
	dropletsInfo, err := getDropletsFromApi(runningConf.forceUpdate)
	if err != nil {
		fmt.Printf("Unable to get droplets. Error: %s\n", err.Error())
		return
	} else if len(dropletsInfo) == 0 {
		fmt.Println("No droplets found in account")
		return
	}

	// Convert all filter expression string to lower
	for i := range filterExpresions {
		filterExpresions[i] = strings.ToLower(filterExpresions[i])
	}

	matchedDroplets := make([]DropletInfo, 0)
	for _, di := range dropletsInfo {
		dropletNameLower := strings.ToLower(di.Name)

		netAdd := di.getInterfaceAddresses()
		publicIpAddressesString := strings.Join(netAdd.publicIps, ", ")
		privateIpAddressesString := strings.Join(netAdd.privateIps, ", ")

		if len(filterExpresions) > 0 {
			filterMatched := false

			for _, fe := range filterExpresions {
				if strings.Contains(dropletNameLower, fe) || strings.Contains(publicIpAddressesString, fe) || strings.Contains(privateIpAddressesString, fe) {
					filterMatched = true
					break
				}
			}

			if !filterMatched {
				continue
			}
		}

		matchedDroplets = append(matchedDroplets, di)
	}

	// Only list public Ips
	if runningConf.listPublicIps {
		for _, di := range matchedDroplets {
			netAdd := di.getInterfaceAddresses()
			publicIpAddressesString := strings.Join(netAdd.publicIps, ", ")

			fmt.Println(publicIpAddressesString)
		}
	} else {
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Id", "Name", "Public IP", "Private IP"})

		for _, di := range matchedDroplets {
			netAdd := di.getInterfaceAddresses()
			publicIpAddressesString := strings.Join(netAdd.publicIps, ", ")
			privateIpAddressesString := strings.Join(netAdd.privateIps, ", ")

			table.Append([]string{strconv.Itoa(di.Id), di.Name, publicIpAddressesString, privateIpAddressesString})
		}

		table.Render()
	}
	fmt.Printf("Total droplets: %d\n", len(matchedDroplets))
}

func printCompletions(runningConf CommandConfig) {
	if flag.NArg() != 2 {
		fmt.Println("Not enough arguments")
		return
	}

	completionFilter := flag.Arg(1)
	droplets, err := getDropletsFromApi(runningConf.forceUpdate)
	if err != nil {
		return
	}

	compFilterLower := strings.ToLower(completionFilter)
	for _, di := range droplets {
		dropletName := di.Name
		dropletNameLower := strings.ToLower(dropletName)

		if strings.Contains(dropletNameLower, compFilterLower) {
			fmt.Println(dropletName)
		}
	}
}
