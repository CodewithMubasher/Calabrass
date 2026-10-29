#!/usr/bin/env node

// Comprehensive test script for Calabrass CLI command handling
// Tests version, help, uninstall commands and argument parsing

const fs = require("fs");
const path = require("path");
const { spawn } = require("child_process");

const CLI_SCRIPT = path.join(__dirname, "bin", "cal.js");
const PACKAGE_JSON = path.join(__dirname, "package.json");

// Test configurations
const testCases = [
    {
        name: "Version command parsing",
        command: "node bin/cal.js --version",
        expected: {
            stdoutContains: true,
            shouldExitClean: true,
            shouldNotStartBackend: true
        },
        validation: (output, error) => {
            if (error) throw new Error(`Command failed: ${error}`);
            const version = output.trim();
            if (!version || version === "unknown") {
                throw new Error("Version should not be 'unknown'");
            }
            console.log(`✓ Version: ${version}`);
        }
    },
    {
        name: "Short version flag",
        command: "node bin/cal.js -v",
        expected: {
            stdoutContains: true,
            shouldExitClean: true,
            shouldNotStartBackend: true
        },
        validation: (output, error) => {
            if (error) throw new Error(`Command failed: ${error}`);
            const version = output.trim();
            if (!version || version === "unknown") {
                throw new Error("Version should not be 'unknown'");
            }
            console.log(`✓ Version (short): ${version}`);
        }
    },
    {
        name: "Version as word",
        command: "node bin/cal.js version",
        expected: {
            stdoutContains: true,
            shouldExitClean: true,
            shouldNotStartBackend: true
        },
        validation: (output, error) => {
            if (error) throw new Error(`Command failed: ${error}`);
            const version = output.trim();
            if (!version || version === "unknown") {
                throw new Error("Version should not be 'unknown'");
            }
            console.log(`✓ Version (word): ${version}`);
        }
    },
    {
        name: "Help command",
        command: "node bin/cal.js --help",
        expected: {
            stdoutContains: ["calabrass", "Usage:", "Examples:"],
            shouldExitClean: true,
            shouldNotStartBackend: true
        },
        validation: (output, error) => {
            if (error) throw new Error(`Command failed: ${error}`);
            const requiredTexts = ["calabrass", "Usage:", "Examples:", "npm uninstall"];
            for (const text of requiredTexts) {
                if (!output.includes(text)) {
                    throw new Error(`Help text missing: ${text}`);
                }
            }
            console.log("✓ Help output contains all required sections");
        }
    },
    {
        name: "Short help flag",
        command: "node bin/cal.js -h",
        expected: {
            stdoutContains: ["calabrass", "Usage:"],
            shouldExitClean: true,
            shouldNotStartBackend: true
        },
        validation: (output, error) => {
            if (error) throw new Error(`Command failed: ${error}`);
            if (!output.includes("calabrass") || !output.includes("Usage:")) {
                throw new Error("Help output incomplete");
            }
            console.log("✓ Short help output correct");
        }
    },
    {
        name: "Uninstall command",
        command: "node bin/cal.js uninstall",
        expected: {
            stdoutContains: ["npm uninstall -g calabrass", "preserve your data"],
            shouldExitClean: true,
            shouldNotStartBackend: true
        },
        validation: (output, error) => {
            if (error) throw new Error(`Command failed: ${error}`);
            const requiredTexts = ["npm uninstall -g calabrass", "preserve your data"];
            for (const text of requiredTexts) {
                if (!output.includes(text)) {
                    throw new Error(`Uninstall help missing: ${text}`);
                }
            }
            console.log("✓ Uninstall instructions correct");
        }
    },
    {
        name: "Unknown command (treated as TUI arg)",
        command: "node bin/cal.js unknown",
        expected: {
            shouldStartBackend: true, // Will try to start backend and TUI
            shouldTreatAsTUI: true
        },
        validation: (output, error) => {
            // This should be treated as a TUI command with "unknown" as an argument
            // The CLI will try to start backend and TUI, which may fail but that's expected
            if (error && !error.includes("calabrass: no TUI binary") && !error.includes("backend exited with code")) {
                throw new Error(`Unexpected error: ${error}`);
            }
            if (output && (output.includes("calabrass: checking backend health") || output.includes("calabrass: starting backend"))) {
                console.log("✓ Unknown command treated as TUI argument, backend started");
            } else if (!error) {
                console.log("✓ Unknown command treated as TUI argument");
            }
        }
    },
    {
        name: "TUI command with flags passthrough",
        command: "node bin/cal.js -addr 127.0.0.1:9999 -c",
        expected: {
            shouldExitClean: false, // Will actually start backend/TUI
            shouldStartBackend: true
        },
        validation: (output, error) => {
            // This command will actually start the backend and TUI, so we expect it to run
            // We'll just verify it doesn't crash immediately and shows appropriate messages
            if (error && !error.includes("calabrass: no TUI binary") && !error.includes("backend exited with code")) {
                throw new Error(`Unexpected error: ${error}`);
            }
            if (output && (output.includes("calabrass: checking backend health") || output.includes("calabrass: starting backend"))) {
                console.log("✓ TUI command with flags initiated backend start");
            } else if (!error) {
                console.log("✓ TUI command with flags accepted");
            }
        }
    },
    {
        name: "Package.json version reading",
        test: () => {
            if (!fs.existsSync(PACKAGE_JSON)) {
                throw new Error("package.json not found");
            }
            const packageData = JSON.parse(fs.readFileSync(PACKAGE_JSON, 'utf8'));
            if (!packageData.version) {
                throw new Error("version not found in package.json");
            }
            return packageData.version;
        },
        expected: {
            shouldReturnValidVersion: true
        },
        validation: (version) => {
            if (!version || version === "unknown") {
                throw new Error("Invalid version from package.json");
            }
            console.log(`✓ Package.json version: ${version}`);
        }
    }
];

// Helper function to run a command and get output
function runCommand(command, options = {}) {
    return new Promise((resolve, reject) => {
        const child = spawn(command.split(' ')[0], command.split(' ').slice(1), {
            ...options,
            cwd: __dirname,
            timeout: 10000 // 10 second timeout
        });
        
        let stdout = "";
        let stderr = "";
        
        child.stdout.on('data', (data) => {
            stdout += data.toString();
        });
        
        child.stderr.on('data', (data) => {
            stderr += data.toString();
        });
        
        child.on('close', (code) => {
            resolve({
                code,
                stdout,
                stderr,
                success: code === 0
            });
        });
        
        child.on('error', (error) => {
            resolve({
                code: -1,
                stdout,
                stderr,
                success: false,
                error
            });
        });
        
        child.on('timeout', () => {
            child.kill();
            resolve({
                code: -1,
                stdout,
                stderr,
                success: false,
                error: new Error('Command timeout')
            });
        });
    });
}

// Run tests
async function runTests() {
    console.log("Running Calabrass CLI command handling tests...\n");
    
    let passed = 0;
    let failed = 0;
    
    for (const testCase of testCases) {
        try {
            console.log(`Testing: ${testCase.name}`);
            
            if (testCase.test) {
                // Direct function test
                const result = testCase.test();
                testCase.validation(result);
            } else {
                // Command test
                const result = await runCommand(testCase.command);
                
                // Check expected conditions
                if (testCase.expected.shouldExitClean !== undefined) {
                    if (testCase.expected.shouldExitClean && !result.success) {
                        throw new Error(`Expected clean exit but got code ${result.code}`);
                    }
                    if (!testCase.expected.shouldExitClean && result.success) {
                        // This is fine for commands that are supposed to run
                    }
                }
                
                if (testCase.expected.shouldExitWithError !== undefined) {
                    if (testCase.expected.shouldExitWithError && result.success) {
                        throw new Error(`Expected error exit but got success`);
                    }
                    if (!testCase.expected.shouldExitWithError && !result.success) {
                        throw new Error(`Expected success exit but got error`);
                    }
                }
                
                if (testCase.expected.stdoutContains) {
                    if (Array.isArray(testCase.expected.stdoutContains)) {
                        for (const text of testCase.expected.stdoutContains) {
                            if (!result.stdout.includes(text)) {
                                throw new Error(`Expected stdout to contain "${text}"`);
                            }
                        }
                    } else if (testCase.expected.stdoutContains === true) {
                        if (!result.stdout.trim()) {
                            throw new Error("Expected stdout but got empty output");
                        }
                    }
                }
                
                if (testCase.expected.stderrContains) {
                    if (Array.isArray(testCase.expected.stderrContains)) {
                        for (const text of testCase.expected.stderrContains) {
                            if (!result.stderr.includes(text)) {
                                throw new Error(`Expected stderr to contain "${text}"`);
                            }
                        }
                    }
                }
                
                // Run validation function if provided
                if (testCase.validation) {
                    testCase.validation(result.stdout, result.stderr || result.error);
                }
            }
            
            console.log(`✓ ${testCase.name} - PASSED\n`);
            passed++;
            
        } catch (error) {
            console.log(`✗ ${testCase.name} - FAILED`);
            console.log(`  Error: ${error.message}\n`);
            failed++;
        }
    }
    
    console.log(`Test Results: ${passed} passed, ${failed} failed`);
    
    if (failed > 0) {
        process.exit(1);
    }
}

// Test CLI script syntax
function testCLIScript() {
    console.log("Testing CLI script syntax...\n");
    
    if (!fs.existsSync(CLI_SCRIPT)) {
        throw new Error("CLI script not found");
    }
    
    const scriptContent = fs.readFileSync(CLI_SCRIPT, 'utf8');
    
    const requiredFunctions = [
        "getVersion",
        "printVersion",
        "printHelp",
        "printUninstallHelp",
        "parseArgs",
        "checkBinaries",
        "checkBackendHealth",
        "startBackend",
        "waitForBackend",
        "launchTUI",
        "main"
    ];
    
    for (const func of requiredFunctions) {
        if (!scriptContent.includes(`function ${func}`) && !scriptContent.includes(`${func} =`)) {
            throw new Error(`Required function not found: ${func}`);
        }
    }
    
    // Check for CLI command definitions
    if (!scriptContent.includes("CLI_COMMANDS")) {
        throw new Error("CLI_COMMANDS not defined");
    }
    
    console.log("✓ CLI script syntax validation - PASSED");
}

// Test package.json
function testPackageJson() {
    console.log("Testing package.json...\n");
    
    if (!fs.existsSync(PACKAGE_JSON)) {
        throw new Error("package.json not found");
    }
    
    const packageData = JSON.parse(fs.readFileSync(PACKAGE_JSON, 'utf8'));
    
    const requiredFields = ["name", "version", "bin", "files"];
    for (const field of requiredFields) {
        if (!(field in packageData)) {
            throw new Error(`Required field missing from package.json: ${field}`);
        }
    }
    
    if (packageData.name !== "calabrass") {
        throw new Error("package.json name should be 'calabrass'");
    }
    
    if (!packageData.bin || !packageData.bin.cal) {
        throw new Error("package.bin.cal not defined");
    }
    
    console.log(`✓ package.json validation - PASSED (version: ${packageData.version})`);
}

// Main test runner
async function main() {
    try {
        // Test script syntax
        testCLIScript();
        
        // Test package.json
        testPackageJson();
        
        // Run individual tests
        await runTests();
        
        console.log("\n🎉 All tests passed! CLI command handling is working correctly.");
        
        // Test specific acceptance criteria commands
        console.log("\n📋 Testing acceptance criteria commands:");
        
        const acceptanceCommands = [
            "node bin/cal.js --version",
            "node bin/cal.js -v", 
            "node bin/cal.js version",
            "node bin/cal.js --help",
            "node bin/cal.js -h",
            "node bin/cal.js uninstall"
        ];
        
        for (const cmd of acceptanceCommands) {
            console.log(`\nTesting: ${cmd}`);
            const result = await runCommand(cmd, { timeout: 5000 });
            if (result.success || result.code === 0) {
                console.log("✓ Command executed successfully");
            } else {
                console.log(`✗ Command failed: ${result.error || result.stderr}`);
            }
        }
        
    } catch (error) {
        console.error(`\n❌ Test failed: ${error.message}`);
        process.exit(1);
    }
}

main();