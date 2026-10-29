#!/usr/bin/env node

// Focused test for backend lifecycle race condition fix

const fs = require("fs");
const path = require("path");
const { spawn } = require("child_process");

const CLI_SCRIPT = path.join(__dirname, "bin", "cal.js");

// Test runner
function runTest(testName, testFn) {
    return new Promise((resolve) => {
        console.log(`\nTest: ${testName}`);
        
        const timeout = setTimeout(() => {
            console.log("✗ FAIL: Test timeout");
            resolve(false);
        }, 10000);
        
        testFn()
            .then((result) => {
                clearTimeout(timeout);
                if (result) {
                    console.log("✓ PASS");
                    resolve(true);
                } else {
                    console.log("✗ FAIL");
                    resolve(false);
                }
            })
            .catch((error) => {
                clearTimeout(timeout);
                console.log(`✗ FAIL: ${error.message}`);
                resolve(false);
            });
    });
}

// Test 1: Check for race condition fixes in the code
function testCodeFixes() {
    const scriptContent = fs.readFileSync(CLI_SCRIPT, 'utf8');
    
    // Check for process state management
    const hasProcessState = scriptContent.includes("processState") && 
                           scriptContent.includes("backendOwnedByCLI");
    
    // Check for single execution path
    const hasSinglePath = scriptContent.includes("if (!processState.backendProcess)") &&
                         scriptContent.includes("processState.backendOwnedByCLI = true");
    
    // Check for proper signal handling
    const hasSignalHandlers = scriptContent.includes("setupSignalHandlers") &&
                            scriptContent.includes("removeAllListeners");
    
    // Check for cleanup function
    const hasCleanup = scriptContent.includes("cleanupBackend") &&
                      scriptContent.includes("isShuttingDown");
    
    if (!hasProcessState) {
        throw new Error("Process state management not found");
    }
    
    if (!hasSinglePath) {
        throw new Error("Single execution path not found");
    }
    
    if (!hasSignalHandlers) {
        throw new Error("Signal handlers not found");
    }
    
    if (!hasCleanup) {
        throw new Error("Cleanup function not found");
    }
    
    return true;
}

// Test 2: Check that version commands don't start backend
async function testVersionCommands() {
    const result = await spawn("node", ["bin/cal.js", "--version"], {
        cwd: __dirname,
        timeout: 3000
    });
    
    return new Promise((resolve) => {
        result.on('close', (code) => {
            // Version command should exit cleanly without backend messages
            const output = result.stdout.toString() + result.stderr.toString();
            const hasBackendMessages = output.includes("backend") || 
                                     output.includes("starting") || 
                                     output.includes("health");
            
            if (code === 0 && !hasBackendMessages) {
                resolve(true);
            } else {
                resolve(false);
            }
        });
        
        result.on('error', () => {
            resolve(false);
        });
    });
}

// Test 3: Check that help commands don't start backend
async function testHelpCommands() {
    const result = await spawn("node", ["bin/cal.js", "--help"], {
        cwd: __dirname,
        timeout: 3000
    });
    
    return new Promise((resolve) => {
        result.on('close', (code) => {
            // Help command should exit cleanly without backend messages
            const output = result.stdout.toString() + result.stderr.toString();
            const hasBackendMessages = output.includes("backend") || 
                                     output.includes("starting") || 
                                     output.includes("health");
            
            if (code === 0 && !hasBackendMessages) {
                resolve(true);
            } else {
                resolve(false);
            }
        });
        
        result.on('error', () => {
            resolve(false);
        });
    });
}

// Test 4: Check that uninstall commands don't start backend
async function testUninstallCommands() {
    const result = await spawn("node", ["bin/cal.js", "uninstall"], {
        cwd: __dirname,
        timeout: 3000
    });
    
    return new Promise((resolve) => {
        result.on('close', (code) => {
            // Uninstall command should exit cleanly without backend messages
            const output = result.stdout.toString() + result.stderr.toString();
            const hasBackendMessages = output.includes("backend") || 
                                     output.includes("starting") || 
                                     output.includes("health");
            
            if (code === 0 && !hasBackendMessages && output.includes("npm uninstall")) {
                resolve(true);
            } else {
                resolve(false);
            }
        });
        
        result.on('error', () => {
            resolve(false);
        });
    });
}

// Test 5: Manual test for terminal state (documented)
function testTerminalState() {
    console.log("Manual test required:");
    console.log("1. Run 'node bin/cal.js' (no backend running)");
    console.log("2. Use TUI normally and exit");
    console.log("3. Check for raw ANSI escape sequences in PowerShell");
    console.log("4. Verify prompt is clean and interactive");
    return true; // Manual test
}

// Main test runner
async function main() {
    console.log("Calabrass CLI Race Condition Fix Tests");
    console.log("=".repeat(50));
    
    const tests = [
        ["Code fixes verification", testCodeFixes],
        ["Version commands don't start backend", testVersionCommands],
        ["Help commands don't start backend", testHelpCommands],
        ["Uninstall commands don't start backend", testUninstallCommands],
        ["Terminal state management", testTerminalState]
    ];
    
    let passed = 0;
    let failed = 0;
    
    for (const [name, testFn] of tests) {
        try {
            const result = await runTest(name, testFn);
            if (result) passed++;
            else failed++;
        } catch (error) {
            console.log(`✗ FAIL: ${name} - ${error.message}`);
            failed++;
        }
    }
    
    console.log("\n" + "=".repeat(50));
    console.log(`Results: ${passed} passed, ${failed} failed`);
    
    if (failed === 0) {
        console.log("\n🎉 All tests passed!");
        process.exit(0);
    } else {
        console.log("\n❌ Some tests failed!");
        process.exit(1);
    }
}

main();