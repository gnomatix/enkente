import * as vscode from 'vscode';
import * as http from 'http';

let enabled = true;
let statusBarItem: vscode.StatusBarItem;
let messageCount = 0;

export function activate(context: vscode.ExtensionContext) {
    console.log('enkente Chat Bridge activated');

    // Status bar indicator
    statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    statusBarItem.command = 'enkente.toggle';
    updateStatusBar();
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);

    // Toggle command
    context.subscriptions.push(
        vscode.commands.registerCommand('enkente.toggle', () => {
            enabled = !enabled;
            updateStatusBar();
            vscode.window.showInformationMessage(
                `enkente Chat Bridge ${enabled ? 'enabled' : 'disabled'}`
            );
        })
    );

    // Status command
    context.subscriptions.push(
        vscode.commands.registerCommand('enkente.status', () => {
            const config = vscode.workspace.getConfiguration('enkente');
            const apiUrl = config.get<string>('apiUrl', 'http://localhost:8080');
            vscode.window.showInformationMessage(
                `enkente: ${enabled ? 'ON' : 'OFF'} | ${messageCount} msgs sent | API: ${apiUrl}`
            );
        })
    );

    // Register as a chat participant to intercept messages
    try {
        const participant = vscode.chat.createChatParticipant('enkente.bridge', handleChatRequest);
        participant.iconPath = new vscode.ThemeIcon('radio-tower');
        context.subscriptions.push(participant);
        console.log('enkente: Registered as chat participant');
    } catch (e) {
        console.log('enkente: Chat participant API not available, falling back to document watcher');
        setupDocumentWatcher(context);
    }

}

async function handleChatRequest(
    request: vscode.ChatRequest,
    _context: vscode.ChatContext,
    stream: vscode.ChatResponseStream,
    _token: vscode.CancellationToken
): Promise<void> {
    if (!enabled) { return; }

    // Forward the user's message
    await postToEnkente('user', request.prompt);

    stream.markdown('*Message forwarded to enkente*');
}

function setupDocumentWatcher(context: vscode.ExtensionContext) {
    // Watch for text document changes as a fallback mechanism
    context.subscriptions.push(
        vscode.workspace.onDidChangeTextDocument((event) => {
            if (!enabled) { return; }

            // Look for chat-related documents
            const uri = event.document.uri.toString();
            if (uri.includes('chat') || uri.includes('conversation')) {
                for (const change of event.contentChanges) {
                    if (change.text.trim().length > 0) {
                        postToEnkente('unknown', change.text.trim());
                    }
                }
            }
        })
    );
}



async function postToEnkente(type: string, message: string): Promise<void> {
    const config = vscode.workspace.getConfiguration('enkente');
    const apiUrl = config.get<string>('apiUrl', 'http://localhost:8080');

    const payload = JSON.stringify({ type, message });
    const url = new URL('/ingest', apiUrl);

    return new Promise((resolve, reject) => {
        const req = http.request(
            {
                hostname: url.hostname,
                port: url.port,
                path: url.pathname,
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Content-Length': Buffer.byteLength(payload),
                },
            },
            (res) => {
                messageCount++;
                updateStatusBar();
                res.resume();
                resolve();
            }
        );

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
    } else {
        statusBarItem.text = `$(circle-slash) enkente: OFF`;
        statusBarItem.tooltip = 'enkente Chat Bridge is disabled — Click to enable';
        statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
    }
}

export function deactivate() {
    console.log('enkente Chat Bridge deactivated');
}
