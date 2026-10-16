import { expect, test } from "bun:test";
import { clockTime, stepText } from "./listen";

test("stepText names the step and the audio part", () => {
  expect(stepText({ step: "page" })).toBe("Reading the page…");
  expect(stepText({ step: "audio", done: 0, total: 3 })).toBe("Recording the audio, part 1 of 3…");
  expect(stepText({ step: "audio", done: 3, total: 3 })).toBe("Recording the audio, part 3 of 3…");
  expect(stepText({ step: "audio", total: 1 })).toBe("Recording the audio…");
  expect(stepText({})).toBe("Working…");
});

test("clockTime formats m:ss and carries a rounded minute", () => {
  expect(clockTime(0)).toBe("0:00");
  expect(clockTime(65.2)).toBe("1:05");
  expect(clockTime(119.6)).toBe("2:00");
  expect(clockTime(866)).toBe("14:26");
});
