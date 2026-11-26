import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import type { Exec, ExecResult } from "../src/session.ts";
import {
	runBatch,
	runCreate,
	runDelete,
	runFrontier,
	runInsert,
	runLink,
	runList,
	runSectionGet,
	runSectionSet,
	runShow,
	runSyncTool,
	runUnlink,
	runUpdate,
	runWhere,
	type ToolOutcome,
} from "../src/tools.ts";

function okResult(out: Partial<ExecResult> = {}): ExecResult {
	return { stdout: "", stderr: "", code: 0, killed: false, ...out };
}

/** Run a tool runner against a capturing Exec and return the argv it passed to pt. */
async function argvOf(run: (exec: Exec) => Promise<ToolOutcome>): Promise<string[]> {
	let args: string[] = [];
	const exec: Exec = async (_command, a) => {
		args = a;
		return okResult({ stdout: "ok" });
	};
	const outcome = await run(exec);
	assert.equal(outcome.ok, true);
	return args;
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

test("runCreate builds argv with type, title, and every optional flag including repeated links", async () => {
	const args = await argvOf((exec) =>
		runCreate(
			exec,
			{
				type: "feature",
				title: "Add search",
				priority: 1,
				mode: "hitl",
				dependsOn: ["aaa11111", "bbb22222"],
				requiredBy: ["ccc33333"],
				relatedTo: ["ddd44444"],
			},
			"/cwd",
		),
	);
	assert.deepEqual(args, [
		"create",
		"feature",
		"Add search",
		"--priority",
		"1",
		"--mode",
		"hitl",
		"--depends-on",
		"aaa11111",
		"--depends-on",
		"bbb22222",
		"--required-by",
		"ccc33333",
		"--related-to",
		"ddd44444",
		"--json",
	]);
});

test("runCreate omits absent optional flags", async () => {
	const args = await argvOf((exec) => runCreate(exec, { type: "chore", title: "Bump" }, "/cwd"));
	assert.deepEqual(args, ["create", "chore", "Bump", "--json"]);
});

test("runShow builds `pt show <id> --json`", async () => {
	const args = await argvOf((exec) => runShow(exec, { id: "ab12cd3" }, "/cwd"));
	assert.deepEqual(args, ["show", "ab12cd3", "--json"]);
});

test("runList builds flags only for the filters given", async () => {
	assert.deepEqual(await argvOf((exec) => runList(exec, {}, "/cwd")), ["list", "--json"]);
	assert.deepEqual(
		await argvOf((exec) => runList(exec, { type: "task", status: "open" }, "/cwd")),
		["list", "--type", "task", "--status", "open", "--json"],
	);
});

test("runUpdate builds flags only for the fields given", async () => {
	assert.deepEqual(await argvOf((exec) => runUpdate(exec, { id: "ab12cd3", status: "closed" }, "/cwd")), ["update", "ab12cd3", "--status", "closed", "--json"]);
	assert.deepEqual(
		await argvOf((exec) => runUpdate(exec, { id: "ab12cd3", title: "New title", priority: 3, mode: "hitl" }, "/cwd")),
		["update", "ab12cd3", "--title", "New title", "--priority", "3", "--mode", "hitl", "--json"],
	);
});

test("runDelete previews without confirm and adds -y only with confirm=true", async () => {
	assert.deepEqual(await argvOf((exec) => runDelete(exec, { id: "ab12cd3" }, "/cwd")), ["delete", "ab12cd3", "--json"]);
	assert.deepEqual(await argvOf((exec) => runDelete(exec, { id: "ab12cd3", confirm: true }, "/cwd")), ["delete", "ab12cd3", "-y", "--json"]);
	assert.deepEqual(await argvOf((exec) => runDelete(exec, { id: "ab12cd3", confirm: false }, "/cwd")), ["delete", "ab12cd3", "--json"]);
});

test("runLink adds exactly the one link direction given", async () => {
	const depends = await argvOf((exec) => runLink(exec, { id: "ab12cd3", dependsOn: "bbb22222" }, "/cwd"));
	assert.deepEqual(depends, ["link", "ab12cd3", "--depends-on", "bbb22222", "--json"]);
	const required = await argvOf((exec) => runLink(exec, { id: "ab12cd3", requiredBy: "bbb22222" }, "/cwd"));
	assert.deepEqual(required, ["link", "ab12cd3", "--required-by", "bbb22222", "--json"]);
	const related = await argvOf((exec) => runLink(exec, { id: "ab12cd3", relatedTo: "bbb22222" }, "/cwd"));
	assert.deepEqual(related, ["link", "ab12cd3", "--related-to", "bbb22222", "--json"]);
});

test("runUnlink builds `pt unlink <id> --json` with the one link direction", async () => {
	assert.deepEqual(
		await argvOf((exec) => runUnlink(exec, { id: "ab12cd3", relatedTo: "bbb22222" }, "/cwd")),
		["unlink", "ab12cd3", "--related-to", "bbb22222", "--json"],
	);
});

test("runInsert builds `pt insert` with after, before, and optional flags", async () => {
	assert.deepEqual(
		await argvOf((exec) => runInsert(exec, { type: "task", title: "Mid", after: "aaa11111", before: "bbb22222" }, "/cwd")),
		["insert", "task", "Mid", "--after", "aaa11111", "--before", "bbb22222", "--json"],
	);
	assert.deepEqual(
		await argvOf((exec) => runInsert(exec, { type: "task", title: "Mid", after: "aaa11111", before: "bbb22222", priority: 0, mode: "hitl" }, "/cwd")),
		["insert", "task", "Mid", "--after", "aaa11111", "--before", "bbb22222", "--priority", "0", "--mode", "hitl", "--json"],
	);
});

test("runFrontier, runSyncTool, and runWhere build their --json commands", async () => {
	assert.deepEqual(await argvOf((exec) => runFrontier(exec, "/cwd")), ["frontier", "--json"]);
	assert.deepEqual(await argvOf((exec) => runSyncTool(exec, "/cwd")), ["sync", "--json"]);
	assert.deepEqual(await argvOf((exec) => runWhere(exec, "/cwd")), ["where", "--json"]);
});

test("runSectionGet builds `pt section get <id> <heading> --json`", async () => {
	assert.deepEqual(await argvOf((exec) => runSectionGet(exec, { id: "ab12cd3", heading: "Outcome" }, "/cwd")), ["section", "get", "ab12cd3", "Outcome", "--json"]);
});

test("the new tool runners report a non-zero exit and a spawn error without throwing", async () => {
	const failing = await runCreate(async () => okResult({ code: 1, stderr: "item: invalid type" }), { type: "nope", title: "X" }, "/cwd");
	assert.equal(failing.ok, false);
	assert.ok(failing.report.includes("exited 1"));
	assert.ok(failing.report.includes("invalid type"));

	const spawn = await runShow(async () => {
		throw new Error("spawn pt ENOENT");
	}, { id: "ab12cd3" }, "/cwd");
	assert.equal(spawn.ok, false);
	assert.ok(spawn.report.includes("spawn pt ENOENT"));
});