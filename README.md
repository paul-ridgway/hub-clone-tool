# Hub Clone Tool
![Build](https://github.com/paul-ridgway/hub-clone-tool/workflows/Build/badge.svg)

## Authentication
Authentication is (currently) by token, stored in the git settings.

To generate a token go to https://github.com/settings/tokens and create a personal access token with the following scopes:

- repo
- read:org

Store the generated token:

`git config --global --add github.apikey [token here]`

## Root Code Folder

By default the tool works out of the current working directory.

To ensure you always sync to the same folder it is advised to set a `code.home` global git config variable:

`git config --global --add code.home /home/paul/Documents/Code`