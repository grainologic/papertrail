import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import type { Exec, ExecResult } from "../src/session.ts";
import { runSectionSet, runBatch } from "../src/tools.ts";

function okResult(out: Partial<ExecResult> = {}): ExecResult {
	return { stdout: "", stderr: "", code: 0, killed: false, ...out };
}

test("runSectionSet writes content to a temp file, hands it to `pt section set --file --json`, and cleans up", async () => {
	let tmpFile = "";
	const exec: Exec = async (command, args) => {
		assert.equal(command, "pt");
		tmpFile = args[5]!;
		assert.deepEqual(args.slice(0, 6), ["section", "set", "ab12cd34", "Goal", "--file", tmpFile]);
		assert.equal(args[6], "--json");
		assert.equal(readFileSync(tmpFile, "utf8"), "Write the parser.");
		return okResult({ stdout: "updated Goal" });
	};
	const outcome = await runSectionSet(exec, { id: "ab12cd34", heading: "Goal", content: "Write the parser." }, "/cwd");
	assert.equal(outcome.ok, true);
	assert.equal(outcome.report, "updated Goal");
	assert.equal(existsSync(tmpFile), false);
});

test("runSectionSet never throws: a spawn error becomes a failed outcome and the temp dir is gone", async () => {
	let tmpFile = "";
	const exec: Exec = async (_command, args) => {
		tmpFile = args[5]!;
		throw new Error("spawn pt ENOENT");
	};
	const outcome = await runSectionSet(exec, { id: "ab12cd34", heading: "Goal", content: "x" }, "/cwd");
	assert.equal(outcome.ok, false);
	assert.ok(outcome.report.includes("spawn pt ENOENT"));
	assert.equal(existsSync(tmpFile), false);
});

test("runSectionSet reports a non-zero exit and a timeout without throwing", async () => {
	const failing = await runSectionSet(
		async (_command, args) => {
			assert.equal(readFileSync(args[5]!, "utf8"), "x");
			return okResult({ code: 1, stderr: "heading not found" });
		},
		{ id: "ab12cd34", heading: "Goal", content: "x" },
		"/cwd",
	);
	assert.equal(failing.ok, false);
	assert.ok(failing.report.includes("exited 1"));
	assert.ok(failing.report.includes("heading not found"));

	const killed = await runSectionSet(async () => okResult({ killed: true }), { id: "ab12cd34", heading: "Goal", content: "x" }, "/cwd", 500);
	assert.equal(killed.ok, false);
	assert.ok(killed.report.includes("terminated"));
	assert.ok(killed.report.includes("500"));
});

test("runBatch writes the items JSON to a temp file, runs `pt batch --json`, and cleans up", async () => {
	let tmpFile = "";
	const items = [
		{ type: "task", title: "Parse" },
		{ type: "bug", title: "Crash", dependsOn: ["a"] },
	];
	const exec: Exec = async (_command, args) => {
		tmpFile = args[1]!;
		assert.deepEqual(args, ["batch", tmpFile, "--json"]);
		assert.equal(readFileSync(tmpFile, "utf8"), JSON.stringify(items));
		return okResult({ stdout: "created 2 items" });
	};
	const outcome = await runBatch(exec, { items }, "/cwd");
	assert.equal(outcome.ok, true);
	assert.equal(outcome.report, "created 2 items");
	assert.equal(existsSync(tmpFile), false);
});

test("runBatch dryRun previews without writing and still cleans up", async () => {
	let tmpFile = "";
	const items = [{ type: "chore", title: "Bump deps" }];
	const exec: Exec = async (_command, args) => {
		tmpFile = args[1]!;
		assert.deepEqual(args, ["batch", tmpFile, "--dry-run", "--json"]);
		return okResult({ stdout: "would create 1 item" });
	};
	const outcome = await runBatch(exec, { items, dryRun: true }, "/cwd");
	assert.equal(outcome.ok, true);
	assert.equal(outcome.report, "would create 1 item");
	assert.equal(existsSync(tmpFile), false);
});

test("runBatch never throws and cleans up when exec throws mid-run", async () => {
	let tmpFile = "";
	const exec: Exec = async (_command, args) => {
		tmpFile = args[1]!;
		throw new Error("spawn pt ENOENT");
	};
	const outcome = await runBatch(exec, { items: [{ type: "task", title: "Parse" }] }, "/cwd");
	assert.equal(outcome.ok, false);
	assert.ok(outcome.report.includes("spawn pt ENOENT"));
	assert.equal(existsSync(tmpFile), false);
});

test("runBatch reports a non-zero exit and a timeout without throwing", async () => {
	const failing = await runBatch(async () => okResult({ code: 2, stderr: "bad json" }), { items: [] }, "/cwd");
	assert.equal(failing.ok, false);
	assert.ok(failing.report.includes("exited 2"));
	assert.ok(failing.report.includes("bad json"));

	const killed = await runBatch(async () => okResult({ killed: true }), { items: [] }, "/cwd", 600);
	assert.equal(killed.ok, false);
	assert.ok(killed.report.includes("terminated"));
	assert.ok(killed.report.includes("600"));
});