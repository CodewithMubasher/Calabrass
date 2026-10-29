const CLI_COMMANDS = {
    version: ["--version", "-v", "version"],
    help: ["--help", "-h", "help"],
    uninstall: ["uninstall"]
};

function parseArgs() {
    const args = process.argv.slice(2);
    
    console.log("Input args:", args);
    
    // Check for CLI commands first
    for (const [command, aliases] of Object.entries(CLI_COMMANDS)) {
        for (const alias of aliases) {
            console.log(`Checking if '${args[0]}' === '${alias}'`);
            if (args[0] === alias) {
                console.log(`Matched command: ${command}`);
                return { command, args: args.slice(1) };
            }
        }
    }
    
    console.log("No CLI command matched, falling back to TUI");
    return { command: "tui", args };
}

// Test with different commands
const testCommands = [
    ["--version"],
    ["-v"],
    ["version"],
    ["--help"],
    ["-h"],
    ["uninstall"],
    ["unknown"],
    ["-addr", "127.0.0.1:8787"]
];

testCommands.forEach(cmd => {
    process.argv = ["node", "test.js", ...cmd];
    const result = parseArgs();
    console.log(`Input: ${cmd} => Result:`, result);
    console.log("---");
});