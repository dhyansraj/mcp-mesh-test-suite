"use client";

import { useState, useEffect } from "react";
import {
  Loader2,
  Save,
  X,
  Settings,
  Package,
  Container,
  Clock,
  FileText,
  CheckCircle,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  getSuiteConfig,
  updateSuiteConfig,
  SuiteConfigStructure,
  SuiteConfigResponse,
} from "@/lib/api";

interface SuiteConfigEditorProps {
  suiteId: number;
  suiteName: string;
  onClose?: () => void;
}

export function SuiteConfigEditor({
  suiteId,
  suiteName,
  onClose,
}: SuiteConfigEditorProps) {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [configData, setConfigData] = useState<SuiteConfigResponse | null>(null);
  const [structure, setStructure] = useState<SuiteConfigStructure | null>(null);
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    loadConfig();
  }, [suiteId]);

  const loadConfig = async () => {
    setLoading(true);
    setError(null);
    setHasChanges(false);
    setSaveSuccess(false);
    try {
      const data = await getSuiteConfig(suiteId);
      setConfigData(data);
      setStructure(data.structure);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load config");
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!structure || !hasChanges) return;

    setSaving(true);
    setError(null);
    setSaveSuccess(false);
    try {
      // Prepare structure for saving
      // When mode is "local", mark version fields for deletion
      const saveStructure = { ...structure };
      if (saveStructure.packages?.mode === "local") {
        saveStructure.packages = {
          ...saveStructure.packages,
          cli_version: "__DELETE__" as string,
          sdk_python_version: "__DELETE__" as string,
          sdk_typescript_version: "__DELETE__" as string,
        };
      }

      await updateSuiteConfig(suiteId, saveStructure);
      setHasChanges(false);
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  // Helper to update nested fields
  const updateNestedField = <K extends keyof SuiteConfigStructure>(
    section: K,
    field: string,
    value: unknown
  ) => {
    if (!structure) return;
    setStructure({
      ...structure,
      [section]: {
        ...(structure[section] as Record<string, unknown>),
        [field]: value,
      },
    });
    setHasChanges(true);
  };

  // Toggle format in reports.formats array
  const toggleFormat = (format: string) => {
    if (!structure) return;
    const formats = structure.reports?.formats || [];
    const newFormats = formats.includes(format)
      ? formats.filter((f) => f !== format)
      : [...formats, format];
    updateNestedField("reports", "formats", newFormats);
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
        <Button variant="outline" onClick={loadConfig} className="mt-4">
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
          <Settings className="h-5 w-5 text-primary" />
          <div>
            <h3 className="font-medium">{suiteName}</h3>
            <p className="text-xs text-muted-foreground">config.yaml</p>
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
            disabled={saving || !hasChanges}
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

      {/* Content */}
      <ScrollArea className="flex-1 h-0">
        <div className="p-4 space-y-6">
        {/* Suite Section */}
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Settings className="h-4 w-4" />
              Suite Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="suite-name">Name</Label>
              <Input
                id="suite-name"
                value={structure.suite?.name || ""}
                onChange={(e) => updateNestedField("suite", "name", e.target.value)}
                placeholder="Suite name"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="suite-mode">Mode</Label>
              <Select
                value={structure.suite?.mode || "docker"}
                onValueChange={(value) => updateNestedField("suite", "mode", value)}
              >
                <SelectTrigger id="suite-mode">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="docker">Docker</SelectItem>
                  <SelectItem value="standalone">Standalone</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </CardContent>
        </Card>

        {/* Packages Section */}
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Package className="h-4 w-4" />
              Package Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="packages-mode">Package Mode</Label>
              <Select
                value={structure.packages?.mode || "auto"}
                onValueChange={(value) => updateNestedField("packages", "mode", value)}
              >
                <SelectTrigger id="packages-mode">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">Auto (detect from image)</SelectItem>
                  <SelectItem value="local">Local (from /wheels, /packages)</SelectItem>
                  <SelectItem value="published">Published (from PyPI/npm)</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {structure.packages?.mode === "local"
                  ? "Uses packages baked into the Docker image (/wheels, /packages)"
                  : structure.packages?.mode === "published"
                  ? "Installs specific versions from PyPI and npm"
                  : "Auto-detects based on presence of /wheels and /packages in container"}
              </p>
            </div>

            {/* Version fields - only show when not in local mode */}
            {structure.packages?.mode !== "local" && (
              <>
                <div className="grid gap-2">
                  <Label htmlFor="cli-version">CLI Version</Label>
                  <Input
                    id="cli-version"
                    value={structure.packages?.cli_version || ""}
                    onChange={(e) => updateNestedField("packages", "cli_version", e.target.value)}
                    placeholder="0.8.0-beta.9"
                    className="font-mono"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="sdk-python-version">SDK Python Version</Label>
                  <Input
                    id="sdk-python-version"
                    value={structure.packages?.sdk_python_version || ""}
                    onChange={(e) => updateNestedField("packages", "sdk_python_version", e.target.value)}
                    placeholder="0.8.0b9"
                    className="font-mono"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="sdk-typescript-version">SDK TypeScript Version</Label>
                  <Input
                    id="sdk-typescript-version"
                    value={structure.packages?.sdk_typescript_version || ""}
                    onChange={(e) => updateNestedField("packages", "sdk_typescript_version", e.target.value)}
                    placeholder="0.8.0-beta.9"
                    className="font-mono"
                  />
                </div>
              </>
            )}
          </CardContent>
        </Card>

        {/* Docker Section */}
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Container className="h-4 w-4" />
              Docker Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="base-image">Base Image</Label>
              <Input
                id="base-image"
                value={structure.docker?.base_image || ""}
                onChange={(e) => updateNestedField("docker", "base_image", e.target.value)}
                placeholder="tsuite-mesh:0.8.0-beta.8"
                className="font-mono"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="network">Network</Label>
              <Input
                id="network"
                value={structure.docker?.network || ""}
                onChange={(e) => updateNestedField("docker", "network", e.target.value)}
                placeholder="bridge"
              />
            </div>
          </CardContent>
        </Card>

        {/* Defaults Section */}
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Execution Defaults
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="max_workers">Max Workers (parallel)</Label>
                <Input
                  id="max_workers"
                  type="number"
                  value={structure.execution?.max_workers || ""}
                  onChange={(e) => updateNestedField("execution", "max_workers", parseInt(e.target.value) || 0)}
                  placeholder="1"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="timeout">Timeout (seconds)</Label>
                <Input
                  id="timeout"
                  type="number"
                  value={structure.execution?.timeout || ""}
                  onChange={(e) => updateNestedField("execution", "timeout", parseInt(e.target.value) || 0)}
                  placeholder="300"
                />
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Reports Section */}
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <FileText className="h-4 w-4" />
              Report Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="output-dir">Output Directory</Label>
              <Input
                id="output-dir"
                value={structure.reports?.output_dir || ""}
                onChange={(e) => updateNestedField("reports", "output_dir", e.target.value)}
                placeholder="./reports"
                className="font-mono"
              />
            </div>
            <div className="grid gap-2">
              <Label>Formats</Label>
              <div className="flex gap-3">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={(structure.reports?.formats || []).includes("html")}
                    onChange={() => toggleFormat("html")}
                    className="h-4 w-4 rounded border-input"
                  />
                  <span className="text-sm">HTML</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={(structure.reports?.formats || []).includes("json")}
                    onChange={() => toggleFormat("json")}
                    className="h-4 w-4 rounded border-input"
                  />
                  <span className="text-sm">JSON</span>
                </label>
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="keep-last">Keep Last N Reports</Label>
              <Input
                id="keep-last"
                type="number"
                value={structure.reports?.keep_last || ""}
                onChange={(e) => updateNestedField("reports", "keep_last", parseInt(e.target.value) || 0)}
                placeholder="10"
                className="w-32"
              />
            </div>
          </CardContent>
        </Card>
        </div>
      </ScrollArea>
    </div>
  );
}
