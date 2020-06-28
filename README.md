# Hub Clone Tool

## Authentication
Authentication is (currently) by token stored in the git settings.

To generate a token go to https://github.com/settings/tokens and create a personal access token with the following scopes:

- repo
- read:org

Store the generated token:

`git config --global --add github.apikey [token here]`