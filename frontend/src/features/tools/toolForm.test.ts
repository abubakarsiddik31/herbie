import { describe, expect, it } from "vitest";
import {
  emptyForm,
  extractUrlPlaceholders,
  formFromDuplicate,
  generateAgentSchema,
  generateCurlSnippet,
  validateForm,
  type ToolForm,
} from "./toolForm";
import type { UserTool } from "@/lib/types";

describe("toolForm utilities", () => {
  it("extracts url placeholders correctly", () => {
    expect(extractUrlPlaceholders("https://api.github.com/repos/{{owner}}/{{repo}}")).toEqual([
      "owner",
      "repo",
    ]);
    expect(extractUrlPlaceholders("https://api.example.com/item/{{id}}/sub/{{id}}")).toEqual(["id"]);
    expect(extractUrlPlaceholders("https://api.example.com/no-placeholders")).toEqual([]);
  });

  it("duplicates a tool with _copy name", () => {
    const mockTool: UserTool = {
      id: "tool-1",
      name: "weather_tool",
      description: "Get weather",
      method: "GET",
      urlTemplate: "https://weather.example.com/forecast",
      params: [{ name: "city", in: "query", type: "string", required: true, description: "City" }],
      bodyTemplate: "",
      headers: { Authorization: "••••" },
      requireApproval: true,
      enabled: true,
      createdAt: "2026-01-01",
      updatedAt: "2026-01-01",
    };

    const duplicate = formFromDuplicate(mockTool);
    expect(duplicate.name).toBe("weather_tool_copy");
    expect(duplicate.description).toBe("Get weather");
    expect(duplicate.params).toHaveLength(1);
    expect(duplicate.requireApproval).toBe(true);

    const dup2 = formFromDuplicate({ ...mockTool, name: "weather_tool_copy" });
    expect(dup2.name).toBe("weather_tool_copy");
  });

  it("generates agent schema and curl snippet", () => {
    const form: ToolForm = {
      name: "get_user",
      description: "Look up a user by ID",
      method: "GET",
      urlTemplate: "https://api.example.com/users/{{id}}",
      params: [
        { name: "id", in: "path", type: "string", required: true, description: "User ID" },
        { name: "include_profile", in: "query", type: "boolean", required: false, description: "Include profile details" },
      ],
      bodyTemplate: "",
      headers: [{ key: "X-Api-Key", value: "secret123" }],
      requireApproval: false,
      enabled: true,
    };

    const schema = generateAgentSchema(form);
    expect(schema.name).toBe("get_user");
    expect(schema.parameters).toBeDefined();
    const paramsObj = schema.parameters as { required?: string[]; properties: Record<string, unknown> };
    expect(paramsObj.required).toEqual(["id"]);
    expect(paramsObj.properties.id).toEqual({ type: "string", description: "User ID" });

    const curl = generateCurlSnippet(form);
    expect(curl).toContain("curl -X GET");
    expect(curl).toContain("https://api.example.com/users/{id}?include_profile={include_profile}");
    expect(curl).toContain("-H \"X-Api-Key: secret123\"");
  });

  it("validates form fields and path parameters", () => {
    const invalidForm: ToolForm = {
      ...emptyForm(),
      name: "Invalid-Name!",
      urlTemplate: "not-a-url",
    };
    const errors = validateForm(invalidForm);
    expect(errors.name).toBeDefined();

    const pathMismatchForm: ToolForm = {
      name: "valid_name",
      description: "Test desc",
      method: "GET",
      urlTemplate: "https://api.example.com/items/{{item_id}}",
      params: [],
      bodyTemplate: "",
      headers: [],
      requireApproval: false,
      enabled: true,
    };
    const pathErrors = validateForm(pathMismatchForm);
    expect(pathErrors.url).toContain("is used in the URL but not declared");

    const validForm: ToolForm = {
      ...pathMismatchForm,
      params: [{ name: "item_id", in: "path", type: "string", required: true, description: "Item ID" }],
    };
    expect(Object.keys(validateForm(validForm))).toHaveLength(0);
  });
});
