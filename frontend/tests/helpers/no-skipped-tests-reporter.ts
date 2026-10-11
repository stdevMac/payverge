import type {
  FullResult,
  Reporter,
  TestCase,
  TestResult,
} from "@playwright/test/reporter";

/** CI release runs must not silently pass when an advertised journey skips. */
export default class NoSkippedTestsReporter implements Reporter {
  private readonly skipped: string[] = [];

  onTestEnd(test: TestCase, result: TestResult): void {
    if (result.status === "skipped") {
      this.skipped.push(test.titlePath().join(" › "));
    }
  }

  async onEnd(
    _result: FullResult,
  ): Promise<{ status: FullResult["status"] } | void> {
    if (this.skipped.length === 0) return;
    process.stderr.write(
      `Advertised browser journeys skipped (${this.skipped.length}):\n${this.skipped
        .map((title) => `- ${title}`)
        .join("\n")}\n`,
    );
    return { status: "failed" };
  }
}
