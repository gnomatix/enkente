"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = __importStar(require("vscode"));
const http = __importStar(require("http"));
let enabled = true;
let statusBarItem;
let messageCount = 0;
function activate(context) {
    console.log('enkente Chat Bridge activated');
    // Status bar indicator
    statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    statusBarItem.command = 'enkente.toggle';
    updateStatusBar();
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);
    // Toggle command
    context.subscriptions.push(vscode.commands.registerCommand('enkente.toggle', () => {
        enabled = !enabled;
        updateStatusBar();
        vscode.window.showInformationMessage(`enkente Chat Bridge ${enabled ? 'enabled' : 'disabled'}`);
    }));
    // Status command
    context.subscriptions.push(vscode.commands.registerCommand('enkente.status', () => {
        const config = vscode.workspace.getConfiguration('enkente');
        const apiUrl = config.get('apiUrl', 'http://localhost:8080');
        vscode.window.showInformationMessage(`enkente: ${enabled ? 'ON' : 'OFF'} | ${messageCount} msgs sent | API: ${apiUrl}`);
    }));
    // Register as a chat participant to intercept messages
    try {
        const participant = vscode.chat.createChatParticipant('enkente.bridge', handleChatRequest);
        participant.iconPath = new vscode.ThemeIcon('radio-tower');
        context.subscriptions.push(participant);
        console.log('enkente: Registered as chat participant');
    }
    catch (e) {
        console.log('enkente: Chat participant API not available, falling back to document watcher');
        setupDocumentWatcher(context);
    }
}
async function handleChatRequest(request, _context, stream, _token) {
    if (!enabled) {
        return;
    }
    // Forward the user's message
    await postToEnkente('user', request.prompt);
    stream.markdown('*Message forwarded to enkente*');
}
function setupDocumentWatcher(context) {
    // Watch for text document changes as a fallback mechanism
    context.subscriptions.push(vscode.workspace.onDidChangeTextDocument((event) => {
        if (!enabled) {
            return;
        }
        // Look for chat-related documents
        const uri = event.document.uri.toString();
        if (uri.includes('chat') || uri.includes('conversation')) {
            for (const change of event.contentChanges) {
                if (change.text.trim().length > 0) {
                    postToEnkente('unknown', change.text.trim());
                }
            }
        }
    }));
}
async function postToEnkente(type, message) {
    const config = vscode.workspace.getConfiguration('enkente');
    const apiUrl = config.get('apiUrl', 'http://localhost:8080');
    const payload = JSON.stringify({ type, message });
    const url = new URL('/ingest', apiUrl);
    return new Promise((resolve, reject) => {
        const req = http.request({
            hostname: url.hostname,
            port: url.port,
            path: url.pathname,
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(payload),
            },
        }, (res) => {
            messageCount++;
            updateStatusBar();
            res.resume();
            resolve();
        });
        req.on('error', (err) => {
            console.error(`enkente: Failed to POST - ${err.message}`);
            resolve(); // Don't reject, just log — the bridge is non-critical
        });
        req.write(payload);
        req.end();
    });
}
function updateStatusBar() {
    if (enabled) {
        statusBarItem.text = `$(radio-tower) enkente: ${messageCount}`;
        statusBarItem.tooltip = 'enkente Chat Bridge — Click to toggle';
        statusBarItem.backgroundColor = undefined;
    }
    else {
        statusBarItem.text = `$(circle-slash) enkente: OFF`;
        statusBarItem.tooltip = 'enkente Chat Bridge is disabled — Click to enable';
        statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
    }
}
function deactivate() {
    console.log('enkente Chat Bridge deactivated');
}
//# sourceMappingURL=extension.js.map