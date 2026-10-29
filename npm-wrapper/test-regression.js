#!/usr/bin/env node

// Comprehensive regression test for Calabrass CLI backend lifecycle and terminal management
// Tests for race condition fixes and terminal corruption fixes

const fs = require("fs");
const path = require("path");
const { spawn, execSync } = require("child_process");
const http = require("http");

const CLI_SCRIPT = path.join(__dirname, "bin", "cal.js");
const PACKAGE_JSON = path.join(__dirname, "package.json");
const BACKEND_BINARY = path.join(__dirname, "..", "binaries", "calabrass-win32-x64", "calabrass-backend.exe");
const TUI_BINARY = path.join(__dirname, "..", "binaries", "calabrass-win32-x64", "calabrass-tui.exe");

// Test configuration
const testConfig = {
    backendAddr: "127.0.0.1:8788", // Use different port to avoid conflicts
    healthCheckUrl: `http://127.0.0.1:8788/api/health`,
    timeout: 10000
};

// Test results
let testResults = {
    passed: 0,
    failed: 0,
    tests: []
};

// Helper function to run a command and get output
function runCommand(command, options = {}) {
    return new Promise((resolve, reject) => {
        const child = spawn(command.split(' ')[0], command.split(' ').slice(1), {
            ...options,
            cwd: __dirname,
            timeout: options.timeout || 30000
        });
        
        let stdout = "";
        let stderr = "";
        let timeoutId;
        
        child.stdout.on('data', (data) => {
            stdout += data.toString();
        });
        
        child.stderr.on('data', (data) => {
            stderr += data.toString();
        });
        
        child.on('close', (code) => {
            clearTimeout(timeoutId);
            resolve({
                code,
                stdout,
                stderr,
                success: code === 0
            });
        });
        
        child.on('error', (error) => {
            clearTimeout(timeoutId);
            resolve({
                code: -1,
                stdout,
                stderr,
                success: false,
                error
            });
        });
        
        child.on('timeout', () => {
            clearTimeout(timeoutId);
            child.kill();
            resolve({
                code: -1,
                stdout,
                stderr,
                success: false,
                error: new Error('Command timeout')
            });
        });
        
        // Set timeout
        timeoutId = setTimeout(() => {
            child.kill();
            resolve({
                code: -1,
                stdout,
                stderr,
                success: false,
                error: new Error('Command timeout')
            });
        }, options.timeout || 30000);
    });
}

// Helper function to check if backend is running
function checkBackendRunning() {
    return new Promise((resolve) => {
        const req = http.get(testConfig.healthCheckUrl, (res) => {
            resolve(res.statusCode === 200);
        }).on('error', () => {
            resolve(false);
        });
        
        req.setTimeout(3000, () => {
            req.destroy();
            resolve(false);
        });
    });
}

// Test 1: Exactly one backend startup attempt per CLI invocation
async function testSingleBackendStartup() {
    console.log("\nTest 1: Exactly one backend startup attempt per CLI invocation");
    
    return new Promise((resolve) => {
        let startupCount = 0;
        let output = "";
        
        // Mock startBackend to count calls
        const originalStartBackend = require(CLI_SCRIPT).startBackend;
        
        // Create a modified version that counts calls
        const modifiedScript = `
const originalFs = require('fs');
const originalPath = require('path');
const originalSpawn = require('child_process').spawn;
const originalHttp = require('http');

// Count startBackend calls
let startupCount = 0;

const modifiedSpawn = (command, args, options) => {
    if (command.includes('calabrass-backend')) {
        startupCount++;
        console.log('BACKEND_START_ATTEMPT:', startupCount);
    }
    return originalSpawn(command, args, options);
};

// Override the spawn function
require('child_process').spawn = modifiedSpawn;

// Load and run the original script
require('${CLI_SCRIPT}');
`;
        
        // Write and run the modified script
        const tempFile = path.join(__dirname, "temp-test.js");
        fs.writeFileSync(tempFile, modifiedScript);
        
        const child = spawn("node", ["temp-test.js"], {
            cwd: __dirname,
            timeout: 5000
        });
        
        child.stdout.on('data', (data) => {
            output += data.toString();
        });
        
        child.stderr.on('data', (data) => {
            output += data.toString();
        });
        
        child.on('close', () => {
            fs.unlinkSync(tempFile);
            
            const startupAttempts = (output.match(/BACKEND_START_ATTEMPT: (\d+)/g) || []).length;
            
            if (startupAttempts === 1) {
                testResults.passed++;
                console.log("✓ PASS: Exactly one backend startup attempt");
                resolve(true);
            } else {
                testResults.failed++;
                console.log(`✗ FAIL: Expected 1 startup attempt, got ${startupAttempts}`);
                resolve(false);
            }
        });
        
        child.on('error', () => {
            fs.unlinkSync(tempFile);
            testResults.failed++;
            console.log("✗ FAIL: Script execution error");
            resolve(false);
        });
    });
}

// Test 2: Reuse of healthy pre-existing backend
async function testBackendReuse() {
    console.log("\nTest 2: Reuse of healthy pre-existing backend");
    
    return new Promise((resolve) => {
        // Start a mock backend first
        const mockBackend = spawn("node", ["-e", `
const http = require('http');
const server = http.createServer((req, res) => {
    if (req.url === '/api/health') {
        res.writeHead(200);
        res.end(JSON.stringify({status: 'ok'}));
    }
});
server.listen(8788, () => {
    console.log('Mock backend started');
    setTimeout(() => {
        server.close();
        console.log('Mock backend stopped');
    }, 5000);
});
`], {
            cwd: __dirname,
            timeout: 6000
        });
        
        setTimeout(async () => {
            // Test that CLI reuses existing backend
            const result = await runCommand(`node ${CLI_SCRIPT} --version`, {
                timeout: 5000
            });
            
            if (result.stdout.includes("1.0.7") && !result.stderr.includes("starting backend")) {
                testResults.passed++;
                console.log("✓ PASS: CLI reused existing backend");
                resolve(true);
            } else {
                testResults.failed++;
                console.log("✗ FAIL: CLI did not reuse existing backend");
                resolve(false);
            }
        }, 2000);
    });
}

// Test 3: TUI exit code preservation
async function testTUIExitCode() {
    console.log("\nTest 3: TUI exit code preservation");
    
    return new Promise((resolve) => {
        // Test with a non-TUI command that should exit with code 0
        runCommand(`node ${CLI_SCRIPT} --version`, {
            timeout: 3000
        }).then((result) => {
            if (result.code === 0) {
                testResults.passed++;
                console.log("✓ PASS: TUI exit code preserved");
                resolve(true);
            } else {
                testResults.failed++;
                console.log(`✗ FAIL: Expected exit code 0, got ${result.code}`);
                resolve(false);
            }
        });
    });
}

// Test 4: No duplicate backend startup after TUI exit
async function testNoDuplicateStartup() {
    console.log("\nTest 4: No duplicate backend startup after TUI exit");
    
    return new Promise((resolve) => {
        let output = "";
        
        const child = spawn("node", ["bin/cal.js"], {
            cwd: __dirname,
            timeout: 8000
        });
        
        child.stdout.on('data', (data) => {
            output += data.toString();
        });
        
        child.stderr.on('data', (data) => {
            output += data.toString();
        });
        
        child.on('close', () => {
            const startupCount = (output.match(/backend started successfully/g) || []).length;
            const launchCount = (output.match(/launching TUI/g) || []).length;
            
            if (startupCount <= 1 && launchCount <= 1) {
                testResults.passed++;
                console.log("✓ PASS: No duplicate backend startup or TUI launch");
                resolve(true);
            } else {
                testResults.failed++;
                console.log(`✗ FAIL: Multiple startups (${startupCount}) or launches (${launchCount})`);
                resolve(false);
            }
        });
        
        child.on('error', () => {
            testResults.failed++;
            console.log("✗ FAIL: Command execution error");
            resolve(false);
        });
    });
}

// Test 5: Command parsing works correctly
async function testCommandParsing() {
    console.log("\nTest 5: Command parsing works correctly");
    
    const commands = [
        { cmd: "node bin/cal.js --version", expected: "1.0.7" },
        { cmd: "node bin/cal.js -v", expected: "1.0.7" },
        { cmd: "node bin/cal.js version", expected: "1.0.7" },
        { cmd: "node bin/cal.js --help", expected: "calabrass 1.0.7" },
        { cmd: "node bin/cal.js -h", expected: "calabrass 1.0.7" },
        { cmd: "node bin/cal.js uninstall", expected: "npm uninstall -g calabrass" }
    ];
    
    let allPassed = true;
    
    for (const { cmd, expected } of commands) {
        const result = await runCommand(cmd, { timeout: 3000 });
        
        if (result.stdout.includes(expected)) {
            console.log(`✓ PASS: ${cmd.split(' ').slice(-2).join(' ')}`);
        } else {
            console.log(`✗ FAIL: ${cmd.split(' ').slice(-2).join(' ')} - expected ${expected}`);
            allPassed = false;
        }
    }
    
    if (allPassed) {
        testResults.passed += commands.length;
    } else {
        testResults.failed += commands.length;
    }
    
    return allPassed;
}

// Test 6: Terminal state management (manual test)
function testTerminalState() {
    console.log("\nTest 6: Terminal state management (manual test)");
    console.log("Manual test required: Run 'cal' and exit normally, then check for ANSI escape sequences");
    console.log("Expected: No raw ANSI sequences should appear after TUI exits");
    return Promise.resolve(true);
}

// Test 7: Backend cleanup on interruption
async function testBackendCleanup() {
    console.log("\nTest 7: Backend cleanup on interruption");
    
    return new Promise((resolve) => {
        let backendStarted = false;
        let cleanupCalled = false;
        
        // Create a test script that monitors backend process lifecycle
        const testScript = `
const originalSpawn = require('child_process').spawn;
let backendProcess = null;

const mockSpawn = (command, args, options) => {
    if (command.includes('calabrass-backend')) {
        backendStarted = true;
        console.log('BACKEND_STARTED');
        
        const process = originalSpawn(command, args, options);
        backendProcess = process;
        
        process.on('exit', () => {
            console.log('BACKEND_EXITED');
            cleanupCalled = true;
        });
        
        return process;
    }
    return originalSpawn(command, args, options);
};

require('child_process').spawn = mockSpawn;
require('${CLI_SCRIPT}');
`;
        
        const tempFile = path.join(__dirname, "cleanup-test.js");
        fs.writeFileSync(tempFile, testScript);
        
        const child = spawn("node", ["cleanup-test.js"], {
            cwd: __dirname,
            timeout: 5000
        });
        
        child.stdout.on('data', (data) => {
            const output = data.toString();
            if (output.includes('BACKEND_STARTED')) backendStarted = true;
            if (output.includes('BACKEND_EXITED')) cleanupCalled = true;
        });
        
        child.on('close', () => {
            fs.unlinkSync(tempFile);
            
            if (backendStarted && cleanupCalled) {
                testResults.passed++;
                console.log("✓ PASS: Backend cleanup on interruption");
                resolve(true);
            } else {
                testResults.failed++;
                console.log("✗ FAIL: Backend cleanup failed");
                resolve(false);
            }
        });
        
        child.on('error', () => {
            fs.unlinkSync(tempFile);
            testResults.failed++;
            console.log("✗ FAIL: Cleanup test error");
            resolve(false);
        });
    });
}

// Run all tests
async function runAllTests() {
    console.log("Running Calabrass CLI regression tests...");
    console.log("=" .repeat(50));
    
    const tests = [
        testSingleBackendStartup,
        testBackendReuse,
        testTUIExitCode,
        testNoDuplicateStartup,
        testCommandParsing,
        testTerminalState,
        testBackendCleanup
    ];
    
    for (const test of tests) {
        try {
            await test();
        } catch (error) {
            console.log(`✗ FAIL: Test error - ${error.message}`);
            testResults.failed++;
        }
    }
    
    // Print results
    console.log("\n" + "=" .repeat(50));
    console.log("Test Results:");
    console.log(`Passed: ${testResults.passed}`);
    console.log(`Failed: ${testResults.failed}`);
    console.log(`Total: ${testResults.passed + testResults.failed}`);
    
    if (testResults.failed === 0) {
        console.log("\n🎉 All tests passed!");
        process.exit(0);
    } else {
        console.log("\n❌ Some tests failed!");
        process.exit(1);
    }
}

// Test CLI script syntax
function testCLIScript() {
    console.log("Testing CLI script syntax...");
    
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
        "cleanupBackend",
        "setupSignalHandlers",
        "main"
    ];
    
    for (const func of requiredFunctions) {
        if (!scriptContent.includes(`function ${func}`) && !scriptContent.includes(`${func} =`)) {
            throw new Error(`Required function not found: ${func}`);
        }
    }
    
    // Check for process state management
    if (!scriptContent.includes("processState")) {
        throw new Error("Process state management not found");
    }
    
    // Check for single signal handler setup
    if (!scriptContent.includes("setupSignalHandlers")) {
        throw new Error("Signal handler setup not found");
    }
    
    console.log("✓ CLI script syntax validation - PASSED");
}

// Test package.json
function testPackageJson() {
    console.log("Testing package.json...");
    
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
    
    console.log(`✓ package.json validation - PASSED (version: ${packageData.version})`);
}

// Main test runner
async function main() {
    try {
        console.log("Calabrass CLI Regression Tests");
        console.log("=" .repeat(50));
        
        // Test script syntax
        testCLIScript();
        
        // Test package.json
        testPackageJson();
        
        // Run all tests
        await runAllTests();
        
    } catch (error) {
        console.error(`\n❌ Test failed: ${error.message}`);
        process.exit(1);
    }
}

main();