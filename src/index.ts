import { Observable, Subscriber } from "rxjs";
import fs from 'fs';
import simpleGit, { SimpleGit } from 'simple-git';
import { Octokit } from "@octokit/rest"
import parse from 'parse-git-config'
import { info, error } from "./utils/logger";
import inquirer from 'inquirer';
import Listr from 'listr';

interface IRepo {
  org: string;
  name: string;
  git_url: string;
}

const concurrency = 5; // TODO: Add param
const config = parse.sync({ path: "~/.gitconfig" });

if (!config.github?.apikey) {
  error("No API Key found!")
  process.exit(-1);
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
// // Compare: https://developer.github.com/v3/repos/#list-organization-repositories
function listRepos(org: string): Promise<IRepo[]> {
  return octokit.paginate(octokit.repos.listForOrg, { org, type: "all" })
    .then((data): IRepo[] => data.map((repo): IRepo => {
      return ({
        org,
        name: repo.name,
        git_url: repo.ssh_url
      });
    }));
}

function sanitizeOrg(org: string): string {
  return org.trim().toLowerCase();
}

// TODO: Add git config option (code.root or something)
function basePath(): string {
  return process.cwd();
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
        .then((): void => listr.add(newCloneTask(listr, repos)))
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

async function processOrgs(orgs: string[]): Promise<void> {
  const allRepos: IRepo[] = [];
  for await (const org of orgs) {
    info(`Fetching Repositories for ${org}...`, "WILL_APPEND")
    const repos = await listRepos(org);
    info(`${repos.length} to clone`, "APPEND")
    allRepos.push(...repos);
  }

  if (allRepos.length === 0) {
    info("You have no repos!")
    return process.exit(-1);
  }


  const pwd = process.cwd();
  const result = await checkWithUser(allRepos.length, pwd);
  if (!result.continue) {
    info("Aborted!")
    return process.exit(0);
  }

  await processRepos(allRepos);
}

async function run(): Promise<void> {
  const orgs = await listOrgs();
  await processOrgs(orgs);
}

run()
  .catch((err): void => { throw (err); });


