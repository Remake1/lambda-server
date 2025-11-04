#!/usr/bin/env node

/**
 * WebSocket Test Script for Lambda Server
 * 
 * This script tests the WebSocket functionality:
 * 1. Registers a test user (or uses existing)
 * 2. Logs in to get JWT token
 * 3. Connects client WebSocket (authenticated)
 * 4. Connects hardware WebSocket (with client UUID)
 * 5. Tests message flow between client and hardware
 * 6. Tests binary image sending from hardware
 */

const WebSocket = require('ws');
const http = require('http');
const https = require('https');

// Configuration
const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const WS_URL = process.env.WS_URL || 'ws://localhost:3000';

// Test user credentials
const TEST_USER = {
  username: `testuser_${Date.now()}`,
  email: `test_${Date.now()}@example.com`,
  password: 'testpassword123'
};

// State
let authToken = null;
let clientUserId = null;

/**
 * Make HTTP request helper
 */
function makeRequest(options, data = null) {
  return new Promise((resolve, reject) => {
    const url = new URL(options.path, BASE_URL);
    const protocol = url.protocol === 'https:' ? https : http;
    
    const reqOptions = {
      hostname: url.hostname,
      port: url.port || (url.protocol === 'https:' ? 443 : 80),
      path: url.pathname + url.search,
      method: options.method || 'GET',
      headers: {
        'Content-Type': 'application/json',
        ...options.headers
      }
    };

    const req = protocol.request(reqOptions, (res) => {
      let body = '';
      res.on('data', (chunk) => body += chunk);
      res.on('end', () => {
        try {
          const parsed = body ? JSON.parse(body) : {};
          resolve({ status: res.statusCode, data: parsed });
        } catch (e) {
          resolve({ status: res.statusCode, data: body });
        }
      });
    });

    req.on('error', reject);
    if (data) {
      req.write(JSON.stringify(data));
    }
    req.end();
  });
}

/**
 * Register a test user
 */
async function registerUser() {
  console.log('📝 Registering test user...');
  const response = await makeRequest({
    method: 'POST',
    path: '/api/v1/auth/register'
  }, TEST_USER);

  if (response.status === 201) {
    console.log('✅ User registered successfully:', response.data.user_id);
    clientUserId = response.data.user_id;
    return true;
  } else if (response.status === 500 && response.data.error?.includes('Failed to create user')) {
    // User might already exist, try to login instead
    console.log('⚠️  User might already exist, will try login...');
    return false;
  } else {
    console.error('❌ Registration failed:', response.data);
    return false;
  }
}

/**
 * Decode JWT token to extract user ID (without verification)
 */
function decodeJWT(token) {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    
    const payload = Buffer.from(parts[1], 'base64').toString('utf8');
    const decoded = JSON.parse(payload);
    return decoded.sub || decoded.subject; // JWT subject field
  } catch (e) {
    return null;
  }
}

/**
 * Login to get JWT token
 */
async function login() {
  console.log('🔐 Logging in...');
  const response = await makeRequest({
    method: 'POST',
    path: '/api/v1/auth/login'
  }, {
    email: TEST_USER.email,
    password: TEST_USER.password
  });

  if (response.status === 200 && response.data.token) {
    authToken = response.data.token;
    // Extract user ID from JWT token if we don't have it from registration
    if (!clientUserId) {
      clientUserId = decodeJWT(authToken);
      if (clientUserId) {
        console.log('✅ User ID extracted from token:', clientUserId);
      }
    }
    console.log('✅ Login successful, token received');
    return true;
  } else {
    console.error('❌ Login failed:', response.data);
    return false;
  }
}

/**
 * Test client WebSocket connection
 */
function testClientWebSocket() {
  return new Promise((resolve, reject) => {
    console.log('\n🔌 Connecting client WebSocket...');
    
    const ws = new WebSocket(`${WS_URL}/ws/client`, {
      headers: {
        'Authorization': `Bearer ${authToken}`
      }
    });

    let messagesReceived = [];

    ws.on('open', () => {
      console.log('✅ Client WebSocket connected');
      
      // Send a test command message
      const testMessage = { command: 'test_command' };
      console.log('📤 Sending test message:', testMessage);
      ws.send(JSON.stringify(testMessage));
    });

    ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        console.log('📥 Client received message:', message);
        messagesReceived.push(message);
      } catch (e) {
        console.log('📥 Client received binary/non-JSON message:', data.toString());
        messagesReceived.push(data);
      }
    });

    ws.on('error', (error) => {
      console.error('❌ Client WebSocket error:', error.message);
      reject(error);
    });

    ws.on('close', (code, reason) => {
      console.log(`🔌 Client WebSocket closed: ${code} - ${reason.toString()}`);
      resolve({ messages: messagesReceived, ws });
    });

    // Close after a delay to allow testing
    setTimeout(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.close();
      }
    }, 2000);
  });
}

/**
 * Test hardware WebSocket connection
 */
function testHardwareWebSocket(clientUUID) {
  return new Promise((resolve, reject) => {
    console.log('\n🔌 Connecting hardware WebSocket...');
    console.log(`   Using client UUID: ${clientUUID}`);
    
    const ws = new WebSocket(`${WS_URL}/ws/hardware/${clientUUID}`);

    let messagesReceived = [];

    ws.on('open', () => {
      console.log('✅ Hardware WebSocket connected');
      
      // Wait a bit, then send a binary image (simulated)
      setTimeout(() => {
        // Create a small fake image (PNG header + some data)
        const fakeImage = Buffer.from([
          0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
          ...Array(100).fill(0x00) // Some fake data
        ]);
        
        console.log('📤 Sending binary image data (size:', fakeImage.length, 'bytes)');
        ws.send(fakeImage);
      }, 500);
    });

    ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        console.log('📥 Hardware received message:', message);
        messagesReceived.push(message);
      } catch (e) {
        console.log('📥 Hardware received binary/non-JSON message');
        messagesReceived.push(data);
      }
    });

    ws.on('error', (error) => {
      console.error('❌ Hardware WebSocket error:', error.message);
      reject(error);
    });

    ws.on('close', (code, reason) => {
      console.log(`🔌 Hardware WebSocket closed: ${code} - ${reason.toString()}`);
      resolve({ messages: messagesReceived, ws });
    });

    // Close after a delay
    setTimeout(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.close();
      }
    }, 3000);
  });
}

/**
 * Test full pairing scenario: client connects first, then hardware
 */
async function testPairingScenario() {
  console.log('\n🧪 Testing full pairing scenario...\n');
  
  if (!clientUserId) {
    console.error('❌ Cannot test pairing: clientUserId not available');
    return { clientMessages: [], hardwareMessages: [] };
  }
  
  return new Promise((resolve) => {
    let clientWs = null;
    let hardwareWs = null;
    let clientMessages = [];
    let hardwareMessages = [];

    // Step 1: Connect client
    console.log('1️⃣  Connecting client...');
    clientWs = new WebSocket(`${WS_URL}/ws/client`, {
      headers: {
        'Authorization': `Bearer ${authToken}`
      }
    });

    clientWs.on('open', () => {
      console.log('✅ Client connected, waiting for hardware...');
      
      // Step 2: Connect hardware after a short delay
      setTimeout(() => {
        console.log('2️⃣  Connecting hardware...');
        hardwareWs = new WebSocket(`${WS_URL}/ws/hardware/${clientUserId}`);

        hardwareWs.on('open', () => {
          console.log('✅ Hardware connected, pairing should be established');
          
          // Step 3: Send message from client to hardware
          setTimeout(() => {
            const clientMessage = { command: 'capture_image' };
            console.log('3️⃣  Client sending message:', clientMessage);
            clientWs.send(JSON.stringify(clientMessage));
          }, 500);

          // Step 4: Send binary image from hardware
          setTimeout(() => {
            const fakeImage = Buffer.from([
              0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
              ...Array(200).fill(0x00)
            ]);
            console.log('4️⃣  Hardware sending binary image (size:', fakeImage.length, 'bytes)');
            hardwareWs.send(fakeImage);
          }, 1000);
        });

        hardwareWs.on('message', (data) => {
          try {
            const msg = JSON.parse(data.toString());
            console.log('📥 Hardware received:', msg);
            hardwareMessages.push(msg);
          } catch (e) {
            console.log('📥 Hardware received binary data');
            hardwareMessages.push(data);
          }
        });

        hardwareWs.on('error', (error) => {
          console.error('❌ Hardware error:', error.message);
        });

        hardwareWs.on('close', () => {
          console.log('🔌 Hardware disconnected');
        });
      }, 1000);
    });

    clientWs.on('message', (data) => {
      try {
        const msg = JSON.parse(data.toString());
        console.log('📥 Client received:', msg);
        clientMessages.push(msg);
        
        // Check if we received image properties
        if (msg.size !== undefined) {
          console.log('✅ Image properties received! Size:', msg.size);
        }
      } catch (e) {
        console.log('📥 Client received binary/non-JSON');
        clientMessages.push(data);
      }
    });

    clientWs.on('error', (error) => {
      console.error('❌ Client error:', error.message);
    });

    clientWs.on('close', () => {
      console.log('🔌 Client disconnected');
    });

    // Cleanup after test
    setTimeout(() => {
      if (clientWs) clientWs.close();
      if (hardwareWs) hardwareWs.close();
      resolve({ clientMessages, hardwareMessages });
    }, 5000);
  });
}

/**
 * Main test runner
 */
async function runTests() {
  console.log('🚀 Starting WebSocket tests...\n');
  console.log(`📍 Server URL: ${BASE_URL}`);
  console.log(`📍 WebSocket URL: ${WS_URL}\n`);

  try {
    // Step 1: Register or login
    const registered = await registerUser();
    if (!registered) {
      // Try to login with existing credentials
      // Note: We'll use a fixed test user for login if registration fails
      console.log('⚠️  Using existing credentials for login...');
    }
    
    // Step 2: Login
    const loginSuccess = await login();
    if (!loginSuccess) {
      console.error('❌ Cannot proceed without authentication token');
      process.exit(1);
    }

    // Step 3: Test individual connections
    console.log('\n' + '='.repeat(50));
    console.log('TEST 1: Client WebSocket Connection');
    console.log('='.repeat(50));
    await testClientWebSocket();

    // Step 4: Test pairing scenario
    console.log('\n' + '='.repeat(50));
    console.log('TEST 2: Full Pairing Scenario');
    console.log('='.repeat(50));
    await testPairingScenario();

    console.log('\n' + '='.repeat(50));
    console.log('✅ All tests completed!');
    console.log('='.repeat(50));

  } catch (error) {
    console.error('\n❌ Test failed:', error);
    process.exit(1);
  }
}

// Run tests
runTests();

