const vscode = require('vscode');
const { spawn, execFile } = require('child_process');
const path = require('path');

async function resolveRepoRoot(api) {
    const activeUri = vscode.window.activeTextEditor?.document?.uri;

    // Prefer the repo that contains the active file, else first repo
    let repo = undefined;
    if (api) {
        if (typeof api.getRepository === 'function' && activeUri) {
            repo = api.getRepository(activeUri) || undefined;
        }
        if (!repo && api.repositories && api.repositories.length > 0) {
            repo = api.repositories[0];
        }
    }

    // Candidate root: Git repo root if available, else workspace folder of active file, else first workspace folder
    let candidateRoot = repo?.rootUri?.fsPath
        || (activeUri ? vscode.workspace.getWorkspaceFolder(activeUri)?.uri?.fsPath : undefined)
        || (vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders[0]?.uri?.fsPath)
        || undefined;

    if (!candidateRoot) return { root: undefined, repo };

    // Guard: normalize and try to upgrade to actual Git toplevel if inside a subdir
    candidateRoot = path.normalize(candidateRoot);
    try {
        const root = await new Promise((resolve, reject) => {
            execFile('git', ['rev-parse', '--show-toplevel'], { cwd: candidateRoot }, (err, stdout) => {
                if (err) return resolve(candidateRoot); // Fall back silently
                const out = stdout.toString().trim();
                resolve(out || candidateRoot);
            });
        });
        return { root, repo };
    } catch {
        return { root: candidateRoot, repo };
    }
}

function activate(context) {
    const disposable = vscode.commands.registerCommand('aic.suggestCommitMessage', async () => {
        const gitExtension = vscode.extensions.getExtension('vscode.git');
        const git = gitExtension && gitExtension.exports;
        const api = git && git.getAPI ? git.getAPI(1) : undefined;

        const { root, repo } = await resolveRepoRoot(api);
        if (!root) {
            vscode.window.showErrorMessage('Open a Git repository or run git init');
            return;
        }

        const child = spawn('aic', [], {
            cwd: root,
            env: { ...process.env, AIC_NON_INTERACTIVE: '1', AIC_DAEMON: '1' }
        });

        let out = '';
        let errOut = '';
        child.stdout.on('data', (d) => (out += d.toString()));
        child.stderr.on('data', (d) => (errOut += d.toString()));
        child.on('error', (err) => {
            vscode.window.showErrorMessage(`aic failed to start: ${err.message}`);
        });
        child.on('close', (code) => {
            if (code !== 0) {
                const msg = errOut && errOut.trim().length > 0 ? errOut.trim() : `aic exited with code ${code}`;
                // Special-case common git error
                if (/not a git repository/i.test(msg)) {
                    vscode.window.showErrorMessage('Open a Git repository or run git init');
                } else {
                    vscode.window.showErrorMessage(`aic failed: ${msg}`);
                }
                return;
            }
            const message = (out || '').trim();
            if (!message) {
                vscode.window.showInformationMessage('No commit message suggestion.');
                return;
            }
            if (repo && repo.inputBox) {
                repo.inputBox.value = message;
            } else {
                vscode.window.showInformationMessage(message);
            }
        });
    });
    context.subscriptions.push(disposable);
}
exports.activate = activate;
function deactivate() {}
exports.deactivate = deactivate;
