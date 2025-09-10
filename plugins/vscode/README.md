# AIC Commit Suggestions VS Code Extension

Provides an inline commit message suggestion in the Source Control panel by
running the `aic` CLI.

## Usage

1. Install and configure the `aic` binary.
2. Stage your changes.
3. In VS Code's Source Control view, invoke **AIC: Suggest Commit Message**
   from the commit input box menu or the command palette.
4. The commit message box is filled with the suggested message.

The extension executes `aic` with `AIC_NON_INTERACTIVE=1`, so it uses the
non-interactive suggestion flow.
