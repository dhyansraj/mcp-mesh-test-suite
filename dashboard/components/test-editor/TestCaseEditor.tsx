"use client";

import { useState, useEffect } from "react";
import dynamic from "next/dynamic";
import {
  Loader2,
  Save,
  X,
  Plus,
  Trash2,
  Clock,
  Tag,
  FileText,
  Terminal,
  Play,
  CheckCircle,
  ChevronDown,
  ChevronRight,
} from "lucide-react";

// Dynamically import Monaco to avoid SSR issues
const MonacoEditor = dynamic(() => import("@monaco-editor/react"), {
  ssr: false,
  loading: () => (
    <div className="h-[200px] flex items-center justify-center bg-muted rounded-md">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
});
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  getTestCaseYaml,
  updateTestCaseYaml,
  updateTestStep,
  addTestStep,
  deleteTestStep,
  TestCaseYaml,
  TestCaseStructure,
  TestStep,
  TestAssertion,
} from "@/lib/api";
import { cn } from "@/lib/utils";

interface TestCaseEditorProps {
  suiteId: number;
  testId: string;
  testName: string;
  onClose?: () => void;
}

export function TestCaseEditor({
  suiteId,
  testId,
  testName,
  onClose,
}: TestCaseEditorProps) {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [yamlData, setYamlData] = useState<TestCaseYaml | null>(null);
  const [structure, setStructure] = useState<TestCaseStructure | null>(null);
  const [originalStructure, setOriginalStructure] = useState<TestCaseStructure | null>(null);
  const [changedFields, setChangedFields] = useState<Set<keyof TestCaseStructure>>(new Set());

  // Tag input state
  const [newTag, setNewTag] = useState("");

  useEffect(() => {
    loadTestCase();
  }, [suiteId, testId]);

  const loadTestCase = async () => {
    setLoading(true);
    setError(null);
    setChangedFields(new Set());
    setSaveSuccess(false);
    try {
      const data = await getTestCaseYaml(suiteId, testId);
      setYamlData(data);
      setStructure(data.structure);
      // Store a deep copy of original for comparison
      setOriginalStructure(JSON.parse(JSON.stringify(data.structure)));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load test case");
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!structure || changedFields.size === 0) return;

    setSaving(true);
    setError(null);
    setSaveSuccess(false);
    try {
      // Only send the fields that have actually changed
      const updates: Partial<TestCaseStructure> = {};
      changedFields.forEach((field) => {
        updates[field] = structure[field] as any;
      });

      await updateTestCaseYaml(suiteId, testId, { updates });
      setChangedFields(new Set());
      // Update original to match current after successful save
      setOriginalStructure(JSON.parse(JSON.stringify(structure)));
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const updateField = <K extends keyof TestCaseStructure>(
    field: K,
    value: TestCaseStructure[K]
  ) => {
    if (!structure) return;
    setStructure({ ...structure, [field]: value });
    // Track which fields have changed
    setChangedFields((prev) => new Set(prev).add(field));
  };

  const addTag = () => {
    if (!newTag.trim() || !structure) return;
    const tags = [...(structure.tags || []), newTag.trim()];
    updateField("tags", tags);
    setNewTag("");
  };

  const removeTag = (index: number) => {
    if (!structure) return;
    const tags = [...(structure.tags || [])];
    tags.splice(index, 1);
    updateField("tags", tags);
  };

  const handleUpdateStep = async (
    phase: "pre_run" | "test" | "post_run",
    index: number,
    updates: Partial<TestStep>
  ) => {
    if (!structure) return;
    try {
      // Call API to update step (preserves YAML comments)
      await updateTestStep(suiteId, testId, phase, index, updates);
      // Update local state for display
      const steps = [...(structure[phase] || [])];
      steps[index] = { ...steps[index], ...updates };
      setStructure({ ...structure, [phase]: steps });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update step");
    }
  };

  const handleAddStep = async (phase: "pre_run" | "test" | "post_run") => {
    if (!structure) return;
    const newStep: TestStep = { name: "New step", handler: "shell", command: "" };
    try {
      // Call API to add step
      await addTestStep(suiteId, testId, phase, newStep);
      // Update local state for display
      const steps = [...(structure[phase] || []), newStep];
      setStructure({ ...structure, [phase]: steps });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add step");
    }
  };

  const handleRemoveStep = async (phase: "pre_run" | "test" | "post_run", index: number) => {
    if (!structure) return;
    try {
      // Call API to delete step
      await deleteTestStep(suiteId, testId, phase, index);
      // Update local state for display
      const steps = [...(structure[phase] || [])];
      steps.splice(index, 1);
      setStructure({ ...structure, [phase]: steps });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete step");
    }
  };

  const updateAssertion = (index: number, updates: Partial<TestAssertion>) => {
    if (!structure) return;
    const assertions = [...(structure.assertions || [])];
    assertions[index] = { ...assertions[index], ...updates };
    updateField("assertions", assertions);
  };

  const addAssertion = () => {
    if (!structure) return;
    const assertions = [...(structure.assertions || [])];
    assertions.push({ expr: "", message: "" });
    updateField("assertions", assertions);
  };

  const removeAssertion = (index: number) => {
    if (!structure) return;
    const assertions = [...(structure.assertions || [])];
    assertions.splice(index, 1);
    updateField("assertions", assertions);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error && !structure) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>{error}</p>
        <Button variant="outline" onClick={loadTestCase} className="mt-4">
          Retry
        </Button>
      </div>
    );
  }

  if (!structure) return null;

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div className="flex items-center gap-3">
          <FileText className="h-5 w-5 text-primary" />
          <div>
            <h3 className="font-medium">{testName}</h3>
            <p className="text-xs text-muted-foreground">{testId}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {saveSuccess && (
            <Badge className="bg-success/20 text-success">
              <CheckCircle className="h-3 w-3 mr-1" />
              Saved
            </Badge>
          )}
          {error && <Badge variant="destructive">{error}</Badge>}
          <Button
            size="sm"
            onClick={handleSave}
            disabled={saving || changedFields.size === 0}
          >
            {saving ? (
              <Loader2 className="h-4 w-4 animate-spin mr-1" />
            ) : (
              <Save className="h-4 w-4 mr-1" />
            )}
            Save
          </Button>
          {onClose && (
            <Button size="sm" variant="ghost" onClick={onClose}>
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Tabs Content */}
      <Tabs defaultValue="metadata" className="flex-1 flex flex-col min-h-0">
        <TabsList className="mx-4 mt-4 w-fit flex-shrink-0">
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
          <TabsTrigger value="steps">
            Steps ({(structure.test?.length || 0)})
          </TabsTrigger>
          <TabsTrigger value="assertions">
            Assertions ({(structure.assertions?.length || 0)})
          </TabsTrigger>
        </TabsList>

        <ScrollArea className="flex-1 h-0">
          <div className="p-4">
          {/* Metadata Tab */}
          <TabsContent value="metadata" className="m-0 space-y-4">
            <div className="grid gap-4">
              {/* Name */}
              <div className="grid gap-2">
                <Label htmlFor="name">Name</Label>
                <Input
                  id="name"
                  value={structure.name || ""}
                  onChange={(e) => updateField("name", e.target.value)}
                  placeholder="Test name"
                />
              </div>

              {/* Description */}
              <div className="grid gap-2">
                <Label htmlFor="description">Description</Label>
                <Textarea
                  id="description"
                  value={structure.description || ""}
                  onChange={(e) => updateField("description", e.target.value)}
                  placeholder="Test description"
                  rows={3}
                />
              </div>

              {/* Timeout */}
              <div className="grid gap-2">
                <Label htmlFor="timeout">Timeout (seconds)</Label>
                <div className="flex items-center gap-2">
                  <Clock className="h-4 w-4 text-muted-foreground" />
                  <Input
                    id="timeout"
                    type="number"
                    value={structure.timeout || 300}
                    onChange={(e) =>
                      updateField("timeout", parseInt(e.target.value) || 300)
                    }
                    className="w-32"
                  />
                </div>
              </div>

              {/* Tags */}
              <div className="grid gap-2">
                <Label>Tags</Label>
                <div className="flex flex-wrap gap-2">
                  {(structure.tags || []).map((tag, index) => (
                    <Badge
                      key={index}
                      variant="secondary"
                      className="gap-1 pr-1"
                    >
                      <Tag className="h-3 w-3" />
                      {tag}
                      <button
                        onClick={() => removeTag(index)}
                        className="ml-1 hover:text-destructive"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                  <div className="flex items-center gap-1">
                    <Input
                      value={newTag}
                      onChange={(e) => setNewTag(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && addTag()}
                      placeholder="Add tag..."
                      className="h-7 w-24 text-xs"
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={addTag}
                      className="h-7 px-2"
                    >
                      <Plus className="h-3 w-3" />
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          </TabsContent>

          {/* Steps Tab */}
          <TabsContent value="steps" className="m-0 space-y-4">
            {/* Pre-run steps */}
            {(structure.pre_run?.length || 0) > 0 && (
              <StepSection
                title="Pre-run Steps"
                steps={structure.pre_run || []}
                phase="pre_run"
                onUpdate={(idx, updates) => handleUpdateStep("pre_run", idx, updates)}
                onAdd={() => handleAddStep("pre_run")}
                onRemove={(idx) => handleRemoveStep("pre_run", idx)}
              />
            )}

            {/* Test steps */}
            <StepSection
              title="Test Steps"
              steps={structure.test || []}
              phase="test"
              onUpdate={(idx, updates) => handleUpdateStep("test", idx, updates)}
              onAdd={() => handleAddStep("test")}
              onRemove={(idx) => handleRemoveStep("test", idx)}
            />

            {/* Post-run steps */}
            {(structure.post_run?.length || 0) > 0 && (
              <StepSection
                title="Post-run Steps"
                steps={structure.post_run || []}
                phase="post_run"
                onUpdate={(idx, updates) =>
                  handleUpdateStep("post_run", idx, updates)
                }
                onAdd={() => handleAddStep("post_run")}
                onRemove={(idx) => handleRemoveStep("post_run", idx)}
              />
            )}
          </TabsContent>

          {/* Assertions Tab */}
          <TabsContent value="assertions" className="m-0 space-y-4">
            <div className="space-y-3">
              {(structure.assertions || []).map((assertion, index) => (
                <Card key={index} className="rounded-md">
                  <CardContent className="p-3">
                    <div className="flex items-start gap-2">
                      <div className="flex-1 space-y-2">
                        <div className="grid gap-1">
                          <Label className="text-xs">Expression</Label>
                          <Input
                            value={assertion.expr || ""}
                            onChange={(e) =>
                              updateAssertion(index, { expr: e.target.value })
                            }
                            placeholder="${captured.var} contains 'value'"
                            className="font-mono text-sm"
                          />
                        </div>
                        <div className="grid gap-1">
                          <Label className="text-xs">Message</Label>
                          <Input
                            value={assertion.message || ""}
                            onChange={(e) =>
                              updateAssertion(index, {
                                message: e.target.value,
                              })
                            }
                            placeholder="Expected value should be present"
                          />
                        </div>
                      </div>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => removeAssertion(index)}
                        className="text-destructive"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
            <Button variant="outline" onClick={addAssertion} className="w-full">
              <Plus className="h-4 w-4 mr-2" />
              Add Assertion
            </Button>
          </TabsContent>
          </div>
        </ScrollArea>
      </Tabs>
    </div>
  );
}

// ============================================================================
// Step Section Component
// ============================================================================

interface StepSectionProps {
  title: string;
  steps: TestStep[];
  phase: "pre_run" | "test" | "post_run";
  onUpdate: (index: number, updates: Partial<TestStep>) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}

function StepSection({
  title,
  steps,
  phase,
  onUpdate,
  onAdd,
  onRemove,
}: StepSectionProps) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="font-medium text-sm">{title}</h4>
        <Button size="sm" variant="outline" onClick={onAdd}>
          <Plus className="h-4 w-4 mr-1" />
          Add Step
        </Button>
      </div>

      <div className="space-y-2">
        {steps.map((step, index) => (
          <StepEditor
            key={index}
            step={step}
            index={index}
            onUpdate={(updates) => onUpdate(index, updates)}
            onRemove={() => onRemove(index)}
          />
        ))}
      </div>
    </div>
  );
}

// ============================================================================
// Step Editor Component
// ============================================================================

interface StepEditorProps {
  step: TestStep;
  index: number;
  onUpdate: (updates: Partial<TestStep>) => void;
  onRemove: () => void;
}

function StepEditor({ step, index, onUpdate, onRemove }: StepEditorProps) {
  const [expanded, setExpanded] = useState(false);

  const handlerType = step.handler || step.routine ? "routine" : "shell";

  return (
    <Card className="rounded-md">
      <CardContent className="p-3">
        {/* Step header */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => setExpanded(!expanded)}
            className="flex items-center gap-2 flex-1 text-left"
          >
            <Badge variant="outline" className="font-mono text-xs">
              {index + 1}
            </Badge>
            <Terminal className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm font-medium truncate flex-1">
              {step.name || `Step ${index + 1}`}
            </span>
            <Badge variant="secondary" className="text-xs">
              {step.handler || step.routine || "shell"}
            </Badge>
          </button>
          <Button
            size="sm"
            variant="ghost"
            onClick={onRemove}
            className="text-destructive h-7 w-7 p-0"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>

        {/* Expanded content */}
        {expanded && (
          <div className="mt-3 space-y-3 pt-3 border-t">
            {/* Name */}
            <div className="grid gap-1">
              <Label className="text-xs">Name</Label>
              <Input
                value={step.name || ""}
                onChange={(e) => onUpdate({ name: e.target.value })}
                placeholder="Step name"
              />
            </div>

            {/* Handler type */}
            <div className="grid gap-1">
              <Label className="text-xs">Handler</Label>
              <Select
                value={step.handler || "shell"}
                onValueChange={(value) => onUpdate({ handler: value })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="shell">Shell</SelectItem>
                  <SelectItem value="wait">Wait</SelectItem>
                  <SelectItem value="http">HTTP</SelectItem>
                  <SelectItem value="file">File</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Shell command with Monaco */}
            {step.handler === "shell" || !step.handler ? (
              <div className="grid gap-1">
                <Label className="text-xs">Command</Label>
                <div className="border rounded-md overflow-hidden">
                  <MonacoEditor
                    height="180px"
                    language="shell"
                    theme="vs-dark"
                    value={step.command || ""}
                    onChange={(value) => onUpdate({ command: value || "" })}
                    options={{
                      minimap: { enabled: false },
                      lineNumbers: "on",
                      scrollBeyondLastLine: false,
                      wordWrap: "on",
                      fontSize: 13,
                      tabSize: 2,
                      automaticLayout: true,
                      padding: { top: 8, bottom: 8 },
                    }}
                  />
                </div>
              </div>
            ) : null}

            {/* Wait handler */}
            {step.handler === "wait" && (
              <div className="grid gap-1">
                <Label className="text-xs">Seconds</Label>
                <Input
                  type="number"
                  value={step.seconds || 5}
                  onChange={(e) =>
                    onUpdate({ seconds: parseInt(e.target.value) || 5 })
                  }
                  className="w-32"
                />
              </div>
            )}

            {/* Optional fields */}
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1">
                <Label className="text-xs">Capture variable</Label>
                <Input
                  value={step.capture || ""}
                  onChange={(e) => onUpdate({ capture: e.target.value })}
                  placeholder="output_var"
                  className="font-mono text-sm"
                />
              </div>
              <div className="grid gap-1">
                <Label className="text-xs">Timeout (s)</Label>
                <Input
                  type="number"
                  value={step.timeout || ""}
                  onChange={(e) =>
                    onUpdate({
                      timeout: e.target.value
                        ? parseInt(e.target.value)
                        : undefined,
                    })
                  }
                  placeholder="Default"
                />
              </div>
            </div>

            <div className="grid gap-1">
              <Label className="text-xs">Working directory</Label>
              <Input
                value={step.workdir || ""}
                onChange={(e) => onUpdate({ workdir: e.target.value })}
                placeholder="/workspace"
                className="font-mono text-sm"
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
