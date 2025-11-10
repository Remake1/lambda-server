#!/usr/bin/env node

/**
 * WebSocket Test Script for Lambda Server - AI Integration Test
 * 
 * This script tests the AI integration with Gemini API:
 * 1. Registers a test user (or uses existing)
 * 2. Logs in to get JWT token
 * 3. Connects client WebSocket (authenticated)
 * 4. Sends message with type="leetcode" and language="C++"
 * 5. Connects hardware WebSocket (with client UUID)
 * 6. Sends sample.png image from hardware
 * 7. Verifies AI response is received from server
 */

const WebSocket = require('ws');
const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');

// Configuration
const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const WS_URL = process.env.WS_URL || 'ws://localhost:3000';

// Test user credentials
const TEST_USER = {
  username: `testuser_ai_${Date.now()}`,
  email: `test_ai_${Date.now()}@example.com`,
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
 * Read sample.png image file
 */
function readSampleImage() {
  const imagePath = path.join(__dirname, 'sample.png');
  try {
    const imageData = fs.readFileSync(imagePath);
    console.log(`✅ Loaded sample.png (${imageData.length} bytes)`);
    return imageData;
  } catch (error) {
    console.error('❌ Failed to read sample.png:', error.message);
    throw error;
  }
}

/**
 * Test AI integration with LeetCode type and C++ language
 */
async function testAIIntegration() {
  console.log('\n🧪 Testing AI Integration with Gemini API...\n');
  
  if (!clientUserId) {
    console.error('❌ Cannot test AI integration: clientUserId not available');
    return;
  }

  // Read the sample image
  const imageData = readSampleImage();

  return new Promise((resolve, reject) => {
    let clientWs = null;
    let hardwareWs = null;
    let clientMessages = [];
    let aiResponseReceived = false;

    // Step 1: Connect client
    console.log('1️⃣  Connecting client WebSocket...');
    clientWs = new WebSocket(`${WS_URL}/ws/client`, {
      headers: {
        'Authorization': `Bearer ${authToken}`
      }
    });

    clientWs.on('open', () => {
      console.log('✅ Client WebSocket connected');
      
      // Step 2: Send message with type and language preferences
      setTimeout(() => {
        const preferencesMessage = {
          type: 'leetcode',
          language: 'C++'
        };
        console.log('2️⃣  Client sending preferences:', preferencesMessage);
        clientWs.send(JSON.stringify(preferencesMessage));
      }, 500);

      // Step 3: Connect hardware after a short delay
      setTimeout(() => {
        console.log('3️⃣  Connecting hardware WebSocket...');
        hardwareWs = new WebSocket(`${WS_URL}/ws/hardware/${clientUserId}`);

        hardwareWs.on('open', () => {
          console.log('✅ Hardware WebSocket connected');
          
          // Step 4: Send the actual image file from hardware
          setTimeout(() => {
            console.log(`4️⃣  Hardware sending sample.png image (${imageData.length} bytes)...`);
            hardwareWs.send(imageData);
          }, 1000);
        });

        hardwareWs.on('message', (data) => {
          try {
            const msg = JSON.parse(data.toString());
            console.log('📥 Hardware received:', msg);
          } catch (e) {
            console.log('📥 Hardware received binary/non-JSON message');
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


    // Set up timeout first (90 seconds) - message handler will clear it if AI response arrives early
    let testTimeout = setTimeout(() => {
      if (clientWs) clientWs.close();
      if (hardwareWs) hardwareWs.close();
      
      console.log('\n' + '='.repeat(60));
      console.log('TEST SUMMARY');
      console.log('='.repeat(60));
      console.log('Total messages received:', clientMessages.length);
      console.log('AI response received:', aiResponseReceived ? '✅ YES' : '❌ NO');
      
      if (aiResponseReceived) {
        console.log('✅ Test PASSED - AI integration is working!');
      } else {
        console.log('❌ Test FAILED - AI response not received');
        console.log('   Make sure:');
        console.log('   1. GOOGLE_API_KEY environment variable is set');
        console.log('   2. Gemini API service is initialized');
        console.log('   3. Server is running and accessible');
        console.log('   4. Gemini API call may take 30-60 seconds to complete');
      }
      console.log('='.repeat(60) + '\n');
      
      resolve({ clientMessages, aiResponseReceived });
    }, 150000); // 150 seconds timeout to allow for slow AI processing

    // Message handler - if we receive the AI response, clear the timeout and close after a short delay
    clientWs.on('message', (data) => {
      try {
        const msg = JSON.parse(data.toString());
        console.log('📥 Client received:', JSON.stringify(msg, null, 2));
        clientMessages.push(msg);
        
        // Check if we received AI analysis result
        if (msg.type === 'image_analysis_result') {
          aiResponseReceived = true;
          console.log('\n' + '='.repeat(60));
          console.log('✅ AI ANALYSIS RESULT RECEIVED!');
          console.log('='.repeat(60));
          console.log('Image Size:', msg.payload?.image_size, 'bytes');
          console.log('\nAI Response:');
          console.log('-'.repeat(60));
          if (msg.payload?.ai_result) {
            console.log(msg.payload.ai_result);
          } else {
            console.log('(AI result field is empty)');
          }
          console.log('-'.repeat(60));
          console.log('='.repeat(60) + '\n');
          
          // Clear the timeout and close connections after a short delay
          clearTimeout(testTimeout);
          setTimeout(() => {
            if (clientWs) clientWs.close();
            if (hardwareWs) hardwareWs.close();
            
            console.log('\n' + '='.repeat(60));
            console.log('TEST SUMMARY');
            console.log('='.repeat(60));
            console.log('Total messages received:', clientMessages.length);
            console.log('AI response received: ✅ YES');
            console.log('✅ Test PASSED - AI integration is working!');
            console.log('='.repeat(60) + '\n');
            
            resolve({ clientMessages, aiResponseReceived });
          }, 1000);
          return;
        } else if (msg.type === 'error') {
          console.error('❌ Error received:', msg.payload);
        } else if (msg.type === 'system_status') {
          console.log('ℹ️  System status:', msg.payload?.message);
        }
      } catch (e) {
        console.log('📥 Client received binary/non-JSON message');
        clientMessages.push(data);
      }
    });

    clientWs.on('error', (error) => {
      console.error('❌ Client error:', error.message);
      reject(error);
    });

    clientWs.on('close', () => {
      console.log('🔌 Client disconnected');
    });
  });
}

/**
 * Main test runner
 */
async function runTests() {
  console.log('🚀 Starting AI Integration WebSocket Test...\n');
  console.log(`📍 Server URL: ${BASE_URL}`);
  console.log(`📍 WebSocket URL: ${WS_URL}\n`);

  try {
    // Step 1: Register or login
    const registered = await registerUser();
    if (!registered) {
      console.log('⚠️  Using existing credentials for login...');
    }
    
    // Step 2: Login
    const loginSuccess = await login();
    if (!loginSuccess) {
      console.error('❌ Cannot proceed without authentication token');
      process.exit(1);
    }

    // Step 3: Test AI integration
    console.log('\n' + '='.repeat(60));
    console.log('AI INTEGRATION TEST');
    console.log('='.repeat(60));
    console.log('Test Configuration:');
    console.log('  - Type: leetcode');
    console.log('  - Language: C++');
    console.log('  - Image: sample.png');
    console.log('='.repeat(60) + '\n');
    
    await testAIIntegration();

    console.log('✅ Test completed!');

  } catch (error) {
    console.error('\n❌ Test failed:', error);
    process.exit(1);
  }
}

// Run tests
runTests();

