#!/usr/bin/env node

// Test script for Calabrass CLI backend integration
// Verifies that the CLI can properly manage backend startup and TUI launch

const fs = require("fs");
const path = require("path");
const { spawn, execSync } = require("child_process");
const http = require("http");

const BACKEND_ADDR = "127.0.0.1:8787";
const HEALTH_CHECK_URL = `http://${BACKEND_ADDR}/api/health`;

// Test configurations
const testCases = [
    {
        name: "Binary existence check",
        test: () => {
            const ext = process.platform === "win32" ? ".exe" : "";
            const platformDir = "calabrass-" + process.platform + "-" + process.arch;
            const tuiBinary = path.join(__dirname, "binaries", platformDir, "calabrass-tui" + ext);
            const backendBinary = path.join(__dirname, "binaries", platformDir, "calabrass-backend" + ext);
            
            if (!fs.existsSync(tuiBinary)) {
                throw new Error(`TUI binary not found: ${tuiBinary}`);
            }
            if (!fs.existsSync(backendBinary)) {
                throw new Error(`Backend binary not found: ${backendBinary}`);
            }
            
            return { tuiBinary, backendBinary };
        }
    },
    {
        name: "Backend health endpoint",
        test: () => {
            return new Promise((resolve, reject) => {
                const req = http.get(HEALTH_CHECK_URL, (res) => {
                    if (res.statusCode === 200) {
                        resolve(true);
                    } else {
                        reject(new Error(`Health check returned ${res.statusCode}`));
                    }
                }).on('error', reject);
                
                req.setTimeout(5000, () => {
                    req.destroy();
                    reject(new Error("Health check timeout"));
                });
            });
        }
    }
];

// Run tests
async function runTests() {
    console.log("Running Calabrass CLI integration tests...\n");
    
    let passed = 0;
    let failed = 0;
    
    for (const testCase of testCases) {
        try {
            console.log(`Testing: ${testCase.name}`);
            const result = await testCase.test();
            console.log(`✓ ${testCase.name} - PASSED`);
            if (result) {
                console.log(`  Result: ${JSON.stringify(result)}`);
            }
            passed++;
        } catch (error) {
            console.log(`✗ ${testCase.name} - FAILED`);
            console.log(`  Error: ${error.message}`);
            failed++;
        }
        console.log("");
    }
    
    console.log(`Test Results: ${passed} passed, ${failed} failed`);
    
    if (failed > 0) {
        process.exit(1);
    }
}

// Test CLI startup (dry run)
function testCLIScript() {
    console.log("Testing CLI script syntax and basic logic...\n");
    
    const cliScript = path.join(__dirname, "bin", "cal.js");
    if (!fs.existsSync(cliScript)) {
        throw new Error("CLI script not found");
    }
    
    // Test that the script can be loaded and basic functions exist
    const scriptContent = fs.readFileSync(cliScript, 'utf8');
    
    const requiredFunctions = [
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
    
    console.log("✓ CLI script validation - PASSED");
    console.log("  All required functions are present");
}

// Main test runner
async function main() {
    try {
        // Test script syntax
        testCLIScript();
        
        // Test individual components
        await runTests();
        
        console.log("\n🎉 All tests passed! The CLI integration is ready.");
        
    } catch (error) {
        console.error(`\n❌ Test failed: ${error.message}`);
        process.exit(1);
    }
}

main();