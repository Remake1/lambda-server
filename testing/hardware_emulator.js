#!/usr/bin/env node

/**
 * Hardware Emulator Script for Lambda Server
 * 
 * This script emulates the hardware part of the API.
 * It connects to the server using a client UUID and allows sending files and status updates.
 */

const WebSocket = require('ws');
const fs = require('fs');
const path = require('path');
const readline = require('readline');

// Default Configuration
const DEFAULT_WS_URL = process.env.WS_URL || 'ws://localhost:3000';

// Create readline interface for input
const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
    prompt: 'HARDWARE> '
});

let ws = null;
let currentUuid = null;
let wsUrl = DEFAULT_WS_URL;

/**
 * Connect to the WebSocket server
 */
function connect(url, uuid) {
    if (ws) {
        console.log('Closing existing connection...');
        ws.close();
    }

    const fullUrl = `${url}/ws/hardware/${uuid}`;
    console.log(`Connecting to ${fullUrl}...`);

    ws = new WebSocket(fullUrl);

    ws.on('open', () => {
        console.log('✅ Connected to server!');
        rl.prompt();
    });

    ws.on('message', (data) => {
        try {
            const msg = JSON.parse(data.toString());
            console.log('\n📥 Received:', JSON.stringify(msg, null, 2));
        } catch (e) {
            console.log('\n📥 Received binary/non-JSON message:', data);
        }
        rl.prompt();
    });

    ws.on('error', (error) => {
        console.error('\n❌ WebSocket Error:', error.message);
        rl.prompt();
    });

    ws.on('close', () => {
        console.log('\n🔌 Disconnected from server');
        ws = null;
        rl.prompt();
    });
}

/**
 * Send a file to the server
 */
function sendFile(filePath) {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        console.error('❌ Not connected to server. Please connect first.');
        return;
    }

    try {
        // Resolve path relative to current working directory
        const absolutePath = path.resolve(process.cwd(), filePath);

        if (!fs.existsSync(absolutePath)) {
            console.error(`❌ File not found: ${absolutePath}`);
            return;
        }

        const fileData = fs.readFileSync(absolutePath);
        console.log(`📤 Sending file: ${path.basename(absolutePath)} (${fileData.length} bytes)...`);

        ws.send(fileData);
        console.log('✅ File sent!');
    } catch (error) {
        console.error(`❌ Failed to read/send file: ${error.message}`);
    }
}

/**
 * Send a status update to the server
 */
function sendStatus(statusMsg) {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        console.error('❌ Not connected to server. Please connect first.');
        return;
    }

    const message = {
        type: 'status',
        status: 'active',
        message: statusMsg,
        data: {
            timestamp: Date.now()
        }
    };

    console.log('📤 Sending status update...');
    ws.send(JSON.stringify(message));
    console.log('✅ Status sent!');
}

/**
 * Main input loop
 */
function startInputLoop() {
    console.log('Hardware Emulator Started');
    console.log('-------------------------');

    rl.question(`Enter WebSocket URL [${DEFAULT_WS_URL}]: `, (urlInput) => {
        wsUrl = urlInput.trim() || DEFAULT_WS_URL;

        rl.question('Enter Client UUID: ', (uuidInput) => {
            currentUuid = uuidInput.trim();

            if (!currentUuid) {
                console.error('❌ UUID is required!');
                process.exit(1);
            }

            connect(wsUrl, currentUuid);

            console.log('\nCommands:');
            console.log('  send <filepath>   - Send a file (binary)');
            console.log('  status <message>  - Send a status update (JSON)');
            console.log('  reconnect         - Reconnect to server');
            console.log('  exit              - Exit script');
            console.log('-------------------------');
            rl.prompt();

            rl.on('line', (line) => {
                const input = line.trim();
                const [command, ...args] = input.split(' ');
                const arg = args.join(' ');

                switch (command.toLowerCase()) {
                    case 'send':
                        if (!arg) {
                            console.log('Usage: send <filepath>');
                        } else {
                            sendFile(arg);
                        }
                        break;

                    case 'status':
                        if (!arg) {
                            console.log('Usage: status <message>');
                        } else {
                            sendStatus(arg);
                        }
                        break;

                    case 'reconnect':
                        connect(wsUrl, currentUuid);
                        break;

                    case 'exit':
                    case 'quit':
                        console.log('Exiting...');
                        if (ws) ws.close();
                        process.exit(0);
                        break;

                    case '':
                        break;

                    default:
                        console.log(`Unknown command: ${command}`);
                        break;
                }
                rl.prompt();
            });
        });
    });
}

// Start the script
startInputLoop();
