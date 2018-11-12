package chronyc

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/influxdata/telegraf/testutil"
)

func TestGather(t *testing.T) {
	c := Chrony{
		path: "chronyc",
	}
	// overwriting exec commands with mock commands
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()
	var acc testutil.Accumulator

	c.ChronycCommands = []string{"tracking", "serverstats"}
	err := c.Gather(&acc)
	if err != nil {
		t.Fatal(err)
	}

	tags := map[string]string{
		"command": "tracking",
		"clockId": "chrony",
	}
	fields := map[string]interface{}{
		"refId":            "PPS",
		"refIdHex":         "50505300",
		"stratum":          int64(1),
		"refTime":          1541793798.895264285,
		"systemTimeOffset": -0.000001007,
		"lastOffset":       0.000000291,
		"rmsOffset":        0.000000239,
		"frequency":        -17.957,
		"freqResidual":     0.000,
		"freqSkew":         0.005,
		"rootDelay":        0.000001000,
		"rootDispersion":   0.000010123,
		"updateInterval":   16.0,
		"leapStatus":       "Normal",
	}

	acc.AssertContainsTaggedFields(t, "chronyc", fields, tags)

	fields = map[string]interface{}{
		"ntpPacketsReceived":      int64(191),
		"ntpPacketsDropped":       int64(222),
		"commandPacketsReceived":  int64(183),
		"commandPacketsDropped":   int64(111),
		"clientLogRecordsDropped": int64(231),
	}
	tags = map[string]string{
		"command": "serverstats",
		"clockId": "chrony",
	}
	acc.AssertContainsTaggedFields(t, "chronyc", fields, tags)

}

// fakeExecCommand is a helper function that mock
// the exec.Command call (and call the test binary)
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// TestHelperProcess isn't a real test. It's used to mock exec.Command
// For example, if you run:
// GO_WANT_HELPER_PROCESS=1 go test -test.run=TestHelperProcess -- chrony tracking
// it returns below mockData.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	trackingOut := `50505300,PPS,1,1541793798.895264285,-0.000001007,0.000000291,0.000000239,-17.957,0.000,0.005,0.000001000,0.000010123,16.0,Normal
`
	serverstatsOut := `191,222,183,111,231
`

	args := os.Args

	// Previous arguments are tests stuff, that looks like :
	// /tmp/go-build970079519/…/_test/integration.test -test.run=TestHelperProcess --

	cmd, args := args[3], args[4:]

	if cmd != "chronyc" {
		fmt.Fprint(os.Stdout, "command not found")
		os.Exit(1)
	}

	if args[0] != "-c" || args[1] != "-m" {
		fmt.Println("First arguments shall be -c -m")
		os.Exit(1)
	}
	for _, command := range args[2:] {
		switch command {
		case "tracking":
			fmt.Fprint(os.Stdout, trackingOut)
		case "serverstats":
			fmt.Fprint(os.Stdout, serverstatsOut)
		default:
			fmt.Printf("Unknown chronyc command %q\n", command)
			os.Exit(1)
		}
	}
	os.Exit(0)
}
