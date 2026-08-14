import { describe, it, expect } from "vitest";
import { authHeader } from "./auth";

describe("authHeader", () => {
  it("returns empty object when token is null", () => {
    expect(authHeader(null)).toEqual({});
  });

  it("returns Bearer header when token present", () => {
    expect(authHeader("abc.123.def")).toEqual({ Authorization: "Bearer abc.123.def" });
  });
});
