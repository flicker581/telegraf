package chronyc

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/internal"
	"github.com/influxdata/telegraf/plugins/inputs"
)

var (
	execCommand = exec.Command // execCommand is used to mock commands in tests.
)

type Chrony struct {
//	DNSLookup bool `toml:"dns_lookup"`
	UseSudo bool `toml:"use_sudo"`
	ChronycCommands []string `toml:"chronyc_commands"`
	path      string
}

func (*Chrony) Description() string {
	return "Get standard chrony metrics, requires chronyc executable."
}

func (*Chrony) SampleConfig() string {
	return `
  ## chronyc command list to run. Possible elements:
  ##  - tracking
  ##  - serverstats
  ##  - sources
  ##  - sourcestats
  ##  - ntpdata
  # chronyc_commands = ["tracking", "sources", "sourcestats"]
  # chronyc_commands = ["tracking", "sources", "sourcestats", "ntpdata", "serverstats"]

  ## chronyc requires root access to unix domain socket to perform some commands:
  ##  - serverstats
  ##  - ntpdata
  ## sudo must be configured to allow telegraf user to run chronyc as root to use this setting.
  # use_sudo = false
 
  `
}

func (c *Chrony) Gather(acc telegraf.Accumulator) error {
	if len(c.path) == 0 {
		return errors.New("chronyc not found: verify that chrony is installed and that chronyc is in your PATH")
	}

	name := c.path
	argv := []string{}
	if c.UseSudo {
		name = "sudo"
		argv = append(argv, "-n", c.path)
	}

	argv = append(argv, "-c", "-m")
	argv = append(argv, c.ChronycCommands...)
	
	cmd := execCommand(name, argv...)
	out, err := internal.CombinedOutputTimeout(cmd, time.Second*5)
	if err != nil {
		return fmt.Errorf("failed to run command %s: %s - %s", strings.Join(cmd.Args, " "), err, string(out))
	}
	err = parseChronycOutput(c.ChronycCommands, string(out), acc)
	if err != nil {
		return err
	}

	return nil
}

type formatError struct {
	error
}

type fieldCountError struct {
	error
}

func parseSourcesLine(fields []string) (map[string]interface{}, map[string]string, error) {
    //fmt.Printf("Input >>>%v<<<\n", fields)

	var err error
	var found bool
	var clockRef string
	var clockMode, clockState int
	var stratum, poll, reach, lastRx int64
	var offset, rawOffset, errorMargin float64

	n := len(fields)
	if ( n != 10) {
		return nil, nil, fieldCountError{fmt.Errorf("Got %d instead of 10 fields in source line", n)}
	}

	modes := map[string]int{
		"^": 0,
		"=": 1,
		"#": 2,
		" ": -1,
	}
	states := map[string]int{
		"*": 0,
		"?": 1,
		"x": 2,
		"~": 3,
		"+": 4,
		"-": 5,
		" ": -1,
	}

	for i, field := range fields {
		switch i {
		case 0: clockMode, found = modes[field]
			if (!found) {
				err = fmt.Errorf("Unknown clock mode %s", field)
			}
		case 1: clockState, found = states[field]
			if (!found) {
				err = fmt.Errorf("Unknown clock state %s", field)
			}
		case 2: clockRef = field
		case 3: stratum, err = strconv.ParseInt(field, 10, 0)
		case 4: poll, err = strconv.ParseInt(field, 10, 0)
		case 5: reach, err = strconv.ParseInt(field, 8, 0)
		case 6: lastRx, err = strconv.ParseInt(field, 10, 0)
		case 7: offset, err = strconv.ParseFloat(field, 64)
		case 8: rawOffset, err = strconv.ParseFloat(field, 64)
		case 9: errorMargin, err = strconv.ParseFloat(field, 64)
		}
		if (err != nil) {
			return nil, nil, formatError{err}
		}
	}

	tFields := map[string]interface{}{
		"clockMode": clockMode,
		"clockState": clockState,
		"stratum": stratum,
		"poll": poll,
		"reach": reach,
		"lastRx": lastRx,
		"offset": offset,
		"rawOffset": rawOffset,
		"errorMargin": errorMargin,
	}
	tTags := map[string]string{
		"command": "sources",
		"clockId": clockRef, 
	}

//	fmt.Printf("Source mode: %d, state: %d, ref: %s, stratum: %d, poll: %d, reach: %d, last rx: %d, offset: %e, raw offset: %e, error margin: %e\n", 
//		clockMode, clockState, clockRef, stratum, poll, reach, lastRx, offset, rawOffset, errorMargin)
	return tFields, tTags, nil
}

func parseSourceStatsLine(fields []string) (map[string]interface{}, map[string]string, error) {
    //fmt.Printf("Input >>>%v<<<\n", fields)

	var err error
	var clockRef string
	var np, nr, span int64
	var frequency, freqSkew, offset, stdDev float64

	n := len(fields)
	if ( n != 8) {
		return nil, nil, fieldCountError{fmt.Errorf("Got %d instead of 8 fields in sourcestats line", n)}
	}

	for i, field := range fields {
		switch i {
		case 0: clockRef = field
		case 1: np, err = strconv.ParseInt(field, 10, 64)
		case 2: nr, err = strconv.ParseInt(field, 10, 64)
		case 3: span, err = strconv.ParseInt(field, 10, 64)
		case 4: frequency, err = strconv.ParseFloat(field, 64)
		case 5: freqSkew, err = strconv.ParseFloat(field, 64)
		case 6: offset, err = strconv.ParseFloat(field, 64)
		case 7: stdDev, err = strconv.ParseFloat(field, 64)
		}
		if (err != nil) {
			return nil, nil, formatError{err}
		}
	}

	tFields := map[string]interface{}{
		"np": np,
		"nr": nr,
		"span": span,
		"frequency": frequency,
		"freqSkew": freqSkew,
		"offset": offset,
		"stdDev": stdDev,
	}
	tTags := map[string]string{
		"command": "sourcestats",
		"clockId": clockRef,
	}
	
//	fmt.Printf("SourceStats ref: %s, np: %d, nr: %d, span: %d, frequency: %f, freqSkew: %f, offset: %e, stdDev: %e\n", 
//		clockRef, np, nr, span, frequency, freqSkew, offset, stdDev)
	return tFields, tTags, nil
}

func parseNtpData(fields []string) (map[string]interface{}, map[string]string, error) {
    //fmt.Printf("Input >>>%v<<<\n", fields)

	var err error
	var remoteAddress, remoteAddressHex, localAddress, localAddressHex string
	var leapStatusStr, clockModeStr, refIdHex, refId string
	var remotePort, version, stratum, pollInterval, precision int64
	var pollIntervalSec, precisionSec, rootDelay, rootDispersion float64
	var refTime, offset, peerDelay, peerDispersion, responseTime float64
	var jitterAsymmetry float64
	var ntpTestsA, ntpTestsB, ntpTestsC string
	var interleaved, authenticated, txTimestamping, rxTimestamping string
	var totalTX, totalRX, totalValidRX int64

	n := len(fields)
	if ( n != 33) {
		return nil, nil, fieldCountError{fmt.Errorf("Got %d instead of 33 fields in ntpdata line", n)}
	}

	for i, field := range fields {
		switch i {
		case 0: remoteAddress = field
		case 1: remoteAddressHex = field
		case 2: remotePort, err = strconv.ParseInt(field, 10, 64)
		case 3: localAddress = field
		case 4: localAddressHex = field
		case 5: leapStatusStr = field
		case 6: version, err = strconv.ParseInt(field, 10, 64)
		case 7: clockModeStr = field
		case 8: stratum, err = strconv.ParseInt(field, 10, 64)
		case 9: pollInterval, err = strconv.ParseInt(field, 10, 64)
		case 10: pollIntervalSec, err = strconv.ParseFloat(field, 64)
		case 11: precision, err = strconv.ParseInt(field, 10, 64)
		case 12: precisionSec, err = strconv.ParseFloat(field, 64)
		case 13: rootDelay, err = strconv.ParseFloat(field, 64)
		case 14: rootDispersion, err = strconv.ParseFloat(field, 64)
		case 15: refIdHex = field
		case 16: refId = field
		case 17: refTime, err = strconv.ParseFloat(field, 64)
		case 18: offset, err = strconv.ParseFloat(field, 64)
		case 19: peerDelay, err = strconv.ParseFloat(field, 64)
		case 20: peerDispersion, err = strconv.ParseFloat(field, 64)
		case 21: responseTime, err = strconv.ParseFloat(field, 64)
		case 22: jitterAsymmetry, err = strconv.ParseFloat(field, 64)
		case 23: ntpTestsA = field
		case 24: ntpTestsB = field
		case 25: ntpTestsC = field
		case 26: interleaved = field
		case 27: authenticated = field
		case 28: txTimestamping = field
		case 29: rxTimestamping = field
		case 30: totalTX, err = strconv.ParseInt(field, 10, 64)
		case 31: totalRX, err = strconv.ParseInt(field, 10, 64)
		case 32: totalValidRX, err = strconv.ParseInt(field, 10, 64)
		}
		if (err != nil) {
			return nil, nil, formatError{err}
		}
	}

	tFields := map[string]interface{}{
		"remoteAddress": remoteAddress,
		"remoteAddressHex": remoteAddressHex,
		"remotePort": remotePort,
		"localAddress": localAddress,
		"localAddressHex": localAddressHex,
		"leapStatus": leapStatusStr,
		"version": version,
		"clockModeStr": clockModeStr,
		"stratum": stratum,
		"pollInterval": pollInterval,
		"pollIntervalSec": pollIntervalSec,
		"precision": precision,
		"precisionSec": precisionSec,
		"rootDelay": rootDelay,
		"rootDispersion": rootDispersion,
		"refIdHex": refIdHex,
		"refId": refId,
		"refTime": refTime,
		"offset": offset,
		"peerDelay": peerDelay,
		"peerDispersion": peerDispersion,
		"responseTime": responseTime,
		"jitterAsymmetry": jitterAsymmetry,
		"ntpTestsA": ntpTestsA,
		"ntpTestsB": ntpTestsB,
		"ntpTestsC": ntpTestsC,
		"interleaved": interleaved,
		"authenticated": authenticated,
		"txTimestamping": txTimestamping,
		"rxTimestamping": rxTimestamping,
		"totalTX": totalTX,
		"totalRX": totalRX,
		"totalValidRX": totalValidRX,
	}
	tTags := map[string]string{
		"command": "ntpdata",
		"clockId": remoteAddress,
		"clockIdHex": remoteAddressHex,
	}

//	fmt.Printf("NtpData remoteAddress: %s, remoteAddressHex: %s, remotePort: %d, localAddress: %s, localAddressHex: %s, leapStatusStr: %s, ", 
//		remoteAddress, remoteAddressHex, remotePort, localAddress, localAddressHex, leapStatusStr)
//	fmt.Printf("version: %d, clockMode: %s, stratum: %d, pollInterval: %d, pollIntervalSec: %f, precision: %d, precisionSec: %f, ",
//		version, clockMode, stratum, pollInterval, pollIntervalSec, precision, precisionSec)
//	fmt.Printf("rootDelay: %f, rootDispersion: %f, refIdHex: %s, refId: %s, refTime: %f, offset: %f, peerDelay: %f, peerDispersion: %f, ", 
//		rootDelay, rootDispersion, refIdHex, refId, refTime, offset, peerDelay, peerDispersion)
//	fmt.Printf("responseTime: %f, jitterAsymmetry: %f, ntpTestsA: %s, ntpTestsB: %s, ntpTestsC: %s, interleaved: %s, authenticated: %s, ",
//		responseTime, jitterAsymmetry, ntpTestsA, ntpTestsB, ntpTestsC, interleaved, authenticated)
//	fmt.Printf("txTimestamping: %s, rxTimestamping: %s, totalTX: %d, totalRX: %d, totalValidRX: %d\n", 
//		txTimestamping, rxTimestamping, totalTX, totalRX, totalValidRX)
	return tFields, tTags, nil
}

func parseTrackingLine(fields []string) (map[string]interface{}, map[string]string, error) {
    //fmt.Printf("Input >>>%v<<<\n", fields)

	var err error
	n := len(fields)
	if ( n != 14) {
		return nil, nil, fieldCountError{fmt.Errorf("Got %d instead of 14 fields in tracking line", n)}
	}
	var refId, refIdHex, leapStatusStr string
	var stratum int64
	var refTime, systemTime, lastOffset, rMSOffset, frequency, freqResidual, freqSkew, rootDelay, rootDispersion, updateInterval float64

	for i, field := range fields {
		switch i {
		case 0: refIdHex = field
		case 1: refId = field
		case 2: stratum, err = strconv.ParseInt(field, 10, 0)
		case 3: refTime, err = strconv.ParseFloat(field, 64)
		case 4: systemTime, err = strconv.ParseFloat(field, 64)
		case 5: lastOffset, err = strconv.ParseFloat(field, 64)
		case 6: rMSOffset, err = strconv.ParseFloat(field, 64)
		case 7: frequency, err = strconv.ParseFloat(field, 64)
		case 8: freqResidual, err = strconv.ParseFloat(field, 64)
		case 9: freqSkew, err = strconv.ParseFloat(field, 64)
		case 10: rootDelay, err = strconv.ParseFloat(field, 64)
		case 11: rootDispersion, err = strconv.ParseFloat(field, 64)
		case 12: updateInterval, err = strconv.ParseFloat(field, 64)
		case 13: leapStatusStr = field
		}
		if (err != nil) {
			return nil, nil, formatError{err}
		}
	}

	tFields := map[string]interface{}{
		"refId": refId,
		"refIdHex": refIdHex,
		"stratum": stratum,
		"refTime": refTime,
		"systemTimeOffset": systemTime,
		"lastOffset": lastOffset,
		"rmsOffset": rMSOffset,
		"frequency": frequency,
		"freqResidual": freqResidual,
		"freqSkew": freqSkew,
		"rootDelay": rootDelay,
		"rootDispersion": rootDispersion,
		"updateInterval": updateInterval,
		"leapStatus": leapStatusStr,
	}
	tTags := map[string]string{
		"command": "tracking",
		"clockId": "chrony",
	}
//	fmt.Printf("Tracking refIdHex: %s, refId: %s, stratum: %d, refTime: %f, systemTime: %f, lastOffset: %f, rMSOffset: %f, frequency: %f, freqResidual: %f, freqSkew: %f, rootDelay: %f, rootDispersion: %f, updateInterval: %f, leapStatus: %s\n",
//		refIdHex, refId, stratum, refTime, systemTime, lastOffset, rMSOffset, frequency, freqResidual, freqSkew, rootDelay, rootDispersion, updateInterval, leapStatusStr)
	return tFields, tTags, nil
}

func parseServerStatsLine(fields []string) (map[string]interface{}, map[string]string, error) {
    //fmt.Printf("Input >>>%v<<<\n", fields)

	var err error
	n := len(fields)
	if ( n != 5) {
		return nil, nil, fieldCountError{fmt.Errorf("Got %d instead of 5 fields in serverstats line", n)}
	}
	var ntpPacketsReceived, ntpPacketsDropped, commandPacketsReceived, commandPacketsDropped, clientLogRecordsDropped int64

	for i, field := range fields {
		switch i {
		case 0: ntpPacketsReceived, err = strconv.ParseInt(field, 10, 64)
		case 1: ntpPacketsDropped, err = strconv.ParseInt(field, 10, 64)
		case 2: commandPacketsReceived, err = strconv.ParseInt(field, 10, 64)
		case 3: commandPacketsDropped, err = strconv.ParseInt(field, 10, 64)
		case 4: clientLogRecordsDropped, err = strconv.ParseInt(field, 10, 64)
		}
		if (err != nil) {
			return nil, nil, formatError{err}
		}
	}

	tFields := map[string]interface{}{
		"ntpPacketsReceived": ntpPacketsReceived,
		"ntpPacketsDropped": ntpPacketsDropped,
		"commandPacketsReceived": commandPacketsReceived,
		"commandPacketsDropped": commandPacketsDropped,
		"clientLogRecordsDropped": clientLogRecordsDropped,
	}
	tTags := map[string]string{
		"command": "serverstats",
		"clockId": "chrony",
	}

//	fmt.Printf("ServerStats ntpPacketsReceived: %d, ntpPacketsDropped: %d, commandPacketsReceived: %d, commandPacketsDropped: %d, clientLogRecordsDropped: %d\n",
//		ntpPacketsReceived, ntpPacketsDropped, commandPacketsReceived, commandPacketsDropped, clientLogRecordsDropped)
	return tFields, tTags, nil
}

func parseChronycOutput(cmds []string, out string, acc telegraf.Accumulator) error {

	var tFields map[string]interface{}
	var tTags map[string]string
	
	singleLine := map[string]bool{
		"tracking": true,
		"serverstats": true,
		"sources": false,
		"sourcestats": false,
		"ntpdata": false,
	}

	var cmd string

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	Lines: 
	for len(lines) > 0 {
		line := lines[0]
		
		var err error
		Cmd:
		for {
			if cmd == "" {
				if len(cmds) == 0 {
					break Lines
				}
				cmd = cmds[0]
				if singleLine[cmd] {
					// Next line will belong to the next command
					cmds = cmds[1:]
				}
			}
			//fmt.Printf("Processing command %s\n", cmd)
			fields := strings.Split(line, ",")
			err = nil
			// process the line with given cmd
			switch cmd {
				case "tracking": tFields, tTags, err = parseTrackingLine(fields)
				case "serverstats": tFields, tTags, err = parseServerStatsLine(fields)
				case "sources": tFields, tTags, err = parseSourcesLine(fields)
				case "sourcestats": tFields, tTags, err = parseSourceStatsLine(fields)
				case "ntpdata": tFields, tTags, err = parseNtpData(fields)
				default:
					return fmt.Errorf("Unknown cmd '%s'", cmd)
			}
			switch err.(type) {
			case nil:
				acc.AddFields("chronyc", tFields, tTags)
				if singleLine[cmd] {
					// done with it, what's next
					cmd = ""
				}
				break Cmd
			case fieldCountError:
				if singleLine[cmd] {
					return fmt.Errorf("Wrong field count for mandatory command '%s'", cmd)
				} else {
					// try next command
					cmd = ""
					cmds = cmds[1:]
				}
			default:
				fmt.Printf("Something wrong: %s", err)
			}
		}
		lines = lines[1:]
	}

	if cmd == "" && len(cmds) == 0 && len(lines) > 0 {
		return fmt.Errorf("Commands done, but there is more output: %v\n", lines)
	}
	if len(lines) == 0 {
		for i, cmd := range cmds {
			if singleLine[cmd] {
				return fmt.Errorf("Not enough output for remaining commands: %v\n", cmds[i:])
			}
		}
	}
	return nil
}

func init() {
	c := Chrony{
		ChronycCommands: []string{"tracking", "sources", "sourcestats"},
	}
	path, _ := exec.LookPath("chronyc")
	if len(path) > 0 {
		c.path = path
	}
	inputs.Add("chronyc", func() telegraf.Input {
		return &c
	})
}
