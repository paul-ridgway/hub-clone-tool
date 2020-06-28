import chalk from "chalk";

const appName = chalk.inverse("Hub Clone Tool ");

export declare type LogExtra = "APPEND" | "WILL_APPEND" | undefined

export function info(message: string, extra?: LogExtra): void {
  if (!extra || extra !== "APPEND") {
    process.stdout.write(appName + chalk.bgGreenBright(chalk.black(" I ")) + " ");
  }
  process.stdout.write(chalk.greenBright(message));
  if (!extra || extra !== "WILL_APPEND") {
    process.stdout.write("\n")
  }
}

export function warn(message: string): void {
  process.stdout.write(appName + chalk.bgYellowBright(chalk.black(" W ")) + " " + chalk.yellowBright(message) + "\n");
}

export function error(message: string): void {
  process.stderr.write(appName + chalk.bgRedBright(chalk.black(" E ")) + " " + chalk.redBright(message) + "\n");
}