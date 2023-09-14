#!/usr/bin/env node

import { Observable, Subscriber } from "rxjs";
import fs from 'fs';
import simpleGit, { SimpleGit } from 'simple-git';
import { Octokit } from "@octokit/rest"
import parse from 'parse-git-config'
import { info, error, warn } from "./utils/logger";
import inquirer from 'inquirer';
import Listr from 'listr';
import { cpus } from "os";

interface IRepo {
  org: string;
  name: string;
  git_url: string;
  archived: boolean;
}

const concurrency = cpus().length; // TODO: Add param?
const config = parse.sync({ path: "~/.gitconfig" });

if (!config.github?.apikey) {
  error("No API Key found!")
  process.exit(1);
}

const octokit = new Octokit({
  auth: config.github!.apikey,
});

function listOrgs(): Promise<string[]> {
  info("Listing orgs...");
  return octokit.orgs
    .listForAuthenticatedUser()
    .then(({ data }): string[] => data.map((o): string => o.login))
}

function listMyRepos(): Promise<IRepo[]> {
  info("Fetching user repositories...", "WILL_APPEND")
  return octokit.paginate(octokit.repos.listForAuthenticatedUser, { type: "owner" })
    .then((data): any => {
      info(`${data.length} to clone`, "APPEND");
      return data;
    })
    .then((data): IRepo[] => data.map((repo: any): IRepo => {
      return ({
        org: "personal", // TODO: Param?
        name: repo.name,
        git_url: repo.ssh_url
      });
    }));
}

async function listOrgRepos(org: string): Promise<IRepo[]> {
  const data = await octokit.paginate(octokit.repos.listForOrg, { org, type: "all" });
  return data.map((repo): IRepo => ({
    org,
    name: repo.name,
    git_url: repo.ssh_url!,
    archived: repo.archived,
  }));
}

function sanitizeOrg(org: string): string {
  return org.trim().toLowerCase();
}

function basePath(): string {
  if (!config.code?.home) {
    return process.cwd();
  }

  if (fs.existsSync(config.code.home)) {
    return config.code.home
  }

  error("The configured code path does not exist: " + config.code.home)
  return process.exit(-1);
}

function pathForRepo(repo: IRepo): string {
  return basePath() + "/" + sanitizeOrg(repo.org) + "/" + repo.name
}

async function runClone(observer: Subscriber<string>, repo: IRepo, path: string): Promise<void> {
  const git: SimpleGit = simpleGit()
  await git
    .outputHandler((_, stdout, stderr): void => {
      stdout.on('data', (chunk): void => {
        observer.next(chunk.toString())
      })
      stderr.on('data', (chunk): void => {
        observer.next(chunk.toString())
      })
    })
    .clone(repo.git_url, path, ["--progress"])
}

function skipTarget(task: Listr.ListrTaskWrapper): Promise<void> {
  return new Promise<void>((res, _): void => {
    task.skip("Target exists");
    res();
  })
}

function newCloneTask(listr: Listr, repos: IRepo[]): Listr.ListrTask {
  const repo = repos.shift()!;
  const path = pathForRepo(repo);
  return ({
    title: `Cloning from ${repo.org} / ${repo.name} into ${path}...`,
    task: (ctx: Listr.ListrContext, task: Listr.ListrTaskWrapper): Observable<string> => new Observable<string>((observer): void => {
      const doTask = fs.existsSync(path) ? skipTarget(task) : runClone(observer, repo, path);
      doTask
        .then((): void => {
          if (repos.length > 0) {
            listr.add(newCloneTask(listr, repos))
          }
        })
        .then((): void => observer.complete())
        .catch((err): void => { throw err })
    }),
  });
}

async function processRepos(repos: IRepo[]): Promise<void> {
  const listr = new Listr([], { concurrent: concurrency })
  for (let i = 0; i < concurrency; ++i) {
    if (repos.length === 0) {
      break;
    }
    listr.add(newCloneTask(listr, repos));
  }
  return listr.run()
}

async function checkWithUser(repoCount: number, dir: string): Promise<any> {
  return await inquirer
    .prompt([
      {
        type: 'confirm',
        message: `Are you sure you want to continue and clone ${repoCount} repositories into ${dir}: `,
        name: 'continue',
      }
    ])
}

async function processOrgs(orgs: string[]): Promise<IRepo[]> {
  const allRepos: IRepo[] = [];
  for await (const org of orgs) {
    info(`Fetching Repositories for ${org}...`, "WILL_APPEND")
    const allRepos = await listOrgRepos(org);
    const activeRepos = allRepos.filter((repo): boolean => !repo.archived);
    const archCount = allRepos.length - activeRepos.length;
    const arch = archCount > 0 ? ` (skipping ${archCount} archived)` : "";
    info(`${activeRepos.length} to clone${arch}`, "APPEND")
    allRepos.push(...activeRepos);
  }
  return allRepos;
}

function checkPath(): void {
  if (config.code?.home) {
    if (fs.existsSync(config.code.home)) {
      info("Cloning to: " + config.code.home)
    } else {
      error("The configured code path does not exist: " + config.code.home)
      return process.exit(-1);
    }
  } else {
    warn("No directory is configured as a git global config variable (code.home).")
    warn("Working out of the current directory: " + process.cwd())
  }
}

async function checkRepos(repos: IRepo[]): Promise<void> {
  if (repos.length === 0) {
    info("You have no repos!")
    return process.exit(-1);
  }

  const result = await checkWithUser(repos.length, basePath());
  if (!result.continue) {
    info("Aborted!")
    return process.exit(0);
  }
}

async function run(): Promise<void> {
  checkPath();

  const orgs = await listOrgs();
  const repos = (await listMyRepos()).concat(await processOrgs(orgs));
  await checkRepos(repos);
  await processRepos(repos);

}

run()
  .then((): void => info("Done!"))
  .catch((err): void => { throw (err); })


