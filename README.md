# Hub Clone Tool
![Build](https://github.com/paul-ridgway/hub-clone-tool/workflows/Build/badge.svg)

![Demo](demo.gif "Demo")

## Running

Either run with `npx`:

```
npx hub-clone-tool
```

Or install globally:

```
npm i -g hub-clone-tool
```

And run with: `hct` or `hub-clone-tool`.

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

## Cloning

Cloning is only supported via SSH (not HTTP/S) as there is no means to prompt for credentials.

## TODO
- Show cloned/skipped stats on complete
  - option to view lists?