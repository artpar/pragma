package powershell

// readOnlyCmdlets maps lowercase cmdlet names and aliases that only read data.
// Sourced from TS readOnlyValidation.ts CMDLET_ALLOWLIST.
//
// DELIBERATELY EXCLUDED per TS reference:
//   - get-clipboard: exposes sensitive data like passwords/API keys (#security)
//   - get-command/gcm: triggers module autoload which can execute arbitrary code (#security)
//   - get-help/help/man: triggers module autoload (#security)
//   - sort: collides with sort.exe which can write files (sort /O)
//   - findstr: native Windows exe, handled through different path in TS
var readOnlyCmdlets = map[string]bool{
	"get-childitem": true, "ls": true, "dir": true, "gci": true,
	"get-content": true, "cat": true, "type": true, "gc": true,
	"get-item": true, "gi": true,
	"test-path": true,
	"select-string": true, "sls": true,
	"get-process": true, "gps": true,
	"get-service": true, "gsv": true,
	"get-location": true, "pwd": true, "gl": true,
	"resolve-path": true, "rvpa": true,
	"measure-object": true, "measure": true,
	"get-member": true, "gm": true,
	"format-list": true, "fl": true,
	"format-table": true, "ft": true,
	"format-wide": true, "fw": true,
	"where-object": true, "where": true, "?": true,
	"select-object": true, "select": true,
	"sort-object": true,
	"group-object": true, "group": true,
	"get-date": true,
	"get-host": true,
	"get-alias": true, "gal": true,
	"get-variable": true, "gv": true,
	"get-module": true, "gmo": true,
	"get-psdrive": true, "gdr": true,
	"get-acl": true,
	"get-filehash": true,
	"write-output": true, "echo": true,
	"write-host": true,
	"out-string": true, "oss": true,
	"out-host": true, "oh": true,
	"convertto-json": true,
	"convertfrom-json": true,
	"convertto-csv": true,
	"convertfrom-csv": true,
	"convertto-xml": true,
	"convertto-html": true,
	"convert-path": true, "cvpa": true,
	"join-path": true,
	"split-path": true,
	"test-connection": true,
	"get-itemproperty": true, "gp": true,
	"get-itempropertyvalue": true, "gpv": true,
	"format-hex": true, "fhx": true,
	"get-unique": true, "gu": true,
	"compare-object": true, "diff": true, "compare": true,
	"get-history": true, "ghy": true, "h": true,
	"get-culture": true,
	"get-uiculture": true,
	"get-random": true,
}

// destructiveCmdlets maps lowercase cmdlet names and aliases that modify data.
// Issue #39631: dangerous command detection must cover PowerShell cmdlets.
//
// DELIBERATELY EXCLUDED aliases that collide with native executables on PS Core 6+:
//   - sc: collides with sc.exe (Service Controller), not Set-Content
//   - sort: collides with sort.exe (can write files via /O flag)
//   - curl/wget: collide with native curl.exe/wget on PS Core 6+
//   - set: not a standard PowerShell alias for Set-Variable
//   - ps: collides with native ps command on Unix
var destructiveCmdlets = map[string]bool{
	"remove-item": true, "del": true, "rm": true, "rmdir": true, "rd": true, "ri": true, "erase": true,
	"move-item": true, "mv": true, "move": true, "mi": true,
	"rename-item": true, "ren": true, "rni": true,
	"set-content": true,
	"out-file": true,
	"add-content": true, "ac": true,
	"new-item": true, "mkdir": true, "ni": true, "md": true,
	"clear-content": true, "clc": true,
	"clear-item": true, "cli": true,
	"stop-process": true, "kill": true, "spps": true,
	"restart-service": true,
	"stop-service": true, "spsv": true,
	"start-service": true, "sasv": true,
	"invoke-webrequest": true, "iwr": true,
	"invoke-restmethod": true, "irm": true,
	"start-process": true, "saps": true, "start": true,
	"copy-item": true, "cp": true, "copy": true, "cpi": true,
	"set-item": true, "si": true,
	"set-variable": true, "sv": true,
	"set-location": true, "cd": true, "chdir": true, "sl": true,
	"set-acl": true,
	"set-executionpolicy": true,
	"invoke-expression": true, "iex": true,
	"invoke-command": true, "icm": true,
	"start-job": true, "sajb": true,
	"import-module": true, "ipmo": true,
	"new-object": true,
	"add-type": true,
	"new-pssession": true, "nsn": true,
	"enter-pssession": true, "etsn": true,
	"set-alias": true, "sal": true,
	"new-alias": true, "nal": true,
	"tee-object": true, "tee": true,
	"export-csv": true, "epcsv": true,
	"set-itemproperty": true, "sp": true,
	"remove-itemproperty": true, "rp": true,
}

