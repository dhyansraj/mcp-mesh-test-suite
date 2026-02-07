"use client";

import { useState, useEffect } from "react";
import {
  Loader2,
  Save,
  X,
  Settings,
  Package,
  Container,
  Server,
  Terminal,
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
        const deletions: Record<string, string> = {};
        Object.keys(saveStructure.packages).forEach((key) => {
          if (key.endsWith("_version")) {
            deletions[key] = "__DELETE__";
          }
        });
        saveStructure.packages = {
          ...saveStructure.packages,
          ...deletions,
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
                  <SelectItem value="k8s">Kubernetes</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between">
              <div>
                <Label>Disabled</Label>
                <p className="text-xs text-muted-foreground">Disable the entire suite</p>
              </div>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={structure.suite?.disabled || false}
                  onChange={(e) => updateNestedField("suite", "disabled", e.target.checked)}
                  className="h-4 w-4 rounded border-input"
                />
                <span className="text-sm">{structure.suite?.disabled ? "Yes" : "No"}</span>
              </label>
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

            {/* Version fields - dynamically rendered from config, hidden in local mode */}
            {structure.packages?.mode !== "local" && structure.packages && (
              <>
                {Object.keys(structure.packages)
                  .filter((key) => key.endsWith("_version"))
                  .sort()
                  .map((key) => {
                    // Convert key like "sdk_java_version" to label "SDK Java Version"
                    const ACRONYMS = new Set(["cli", "sdk", "api", "pip", "npm"]);
                    const label = key
                      .replace(/_version$/, "")
                      .split("_")
                      .map((word) => ACRONYMS.has(word) ? word.toUpperCase() : word.charAt(0).toUpperCase() + word.slice(1))
                      .join(" ")
                      + " Version";
                    return (
                      <div key={key} className="grid gap-2">
                        <Label htmlFor={key}>{label}</Label>
                        <Input
                          id={key}
                          value={(structure.packages?.[key] as string) || ""}
                          onChange={(e) => updateNestedField("packages", key, e.target.value)}
                          className="font-mono"
                        />
                      </div>
                    );
                  })}
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

        {/* K8s Section */}
        {structure.suite?.mode === "k8s" && (
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Server className="h-4 w-4" />
              Kubernetes Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="k8s-namespace">Namespace</Label>
              <Input
                id="k8s-namespace"
                value={structure.k8s?.namespace || ""}
                onChange={(e) => updateNestedField("k8s", "namespace", e.target.value)}
                placeholder="tsuite"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="k8s-nfs-server">NFS Server</Label>
              <Input
                id="k8s-nfs-server"
                value={structure.k8s?.nfs_server || ""}
                onChange={(e) => updateNestedField("k8s", "nfs_server", e.target.value)}
                placeholder="10.0.0.50"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="k8s-nfs-path">NFS Path</Label>
              <Input
                id="k8s-nfs-path"
                value={structure.k8s?.nfs_path || ""}
                onChange={(e) => updateNestedField("k8s", "nfs_path", e.target.value)}
                placeholder="/path/to/tests"
                className="font-mono"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="k8s-image">Image</Label>
              <Input
                id="k8s-image"
                value={structure.k8s?.image || ""}
                onChange={(e) => updateNestedField("k8s", "image", e.target.value)}
                placeholder="tsuite-mesh:local"
                className="font-mono"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="k8s-api-url">API URL</Label>
              <Input
                id="k8s-api-url"
                value={structure.k8s?.api_url || ""}
                onChange={(e) => updateNestedField("k8s", "api_url", e.target.value)}
                placeholder="http://10.0.0.50:9999"
                className="font-mono"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="k8s-kubeconfig">Kubeconfig (optional)</Label>
              <Input
                id="k8s-kubeconfig"
                value={structure.k8s?.kubeconfig || ""}
                onChange={(e) => updateNestedField("k8s", "kubeconfig", e.target.value)}
                placeholder="~/.kube/config"
                className="font-mono"
              />
            </div>
          </CardContent>
        </Card>
        )}

        {/* Standalone Settings - shown when mode is standalone */}
        {structure.suite?.mode === "standalone" && (
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Terminal className="h-4 w-4" />
              Standalone Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="standalone-type">Execution Type</Label>
              <Select
                value={structure.standalone?.type || "local"}
                onValueChange={(value) => updateNestedField("standalone", "type", value)}
              >
                <SelectTrigger id="standalone-type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="local">Local</SelectItem>
                  <SelectItem value="remote">Remote SSH</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {structure.standalone?.type === "remote"
                  ? "Execute tests on a remote host via SSH"
                  : "Execute tests directly on this machine"}
              </p>
            </div>
          </CardContent>
        </Card>
        )}

        {/* SSH Settings - shown when standalone + remote */}
        {structure.suite?.mode === "standalone" && structure.standalone?.type === "remote" && (
        <Card className="rounded-md">
          <CardHeader className="py-3 px-4">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Server className="h-4 w-4" />
              SSH Settings
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="ssh-host">Host</Label>
              <Input
                id="ssh-host"
                value={structure.ssh?.host || ""}
                onChange={(e) => updateNestedField("ssh", "host", e.target.value)}
                placeholder="beelink1 or 10.0.0.101"
                className="font-mono"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ssh-runner-dir">Runner Directory</Label>
              <Input
                id="ssh-runner-dir"
                value={structure.ssh?.runner_dir || ""}
                onChange={(e) => updateNestedField("ssh", "runner_dir", e.target.value)}
                placeholder="/tmp/tsuite"
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">
                Where to stage the runner binary on the remote host
              </p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ssh-api-url">API URL</Label>
              <Input
                id="ssh-api-url"
                value={structure.ssh?.api_url || ""}
                onChange={(e) => updateNestedField("ssh", "api_url", e.target.value)}
                placeholder="http://10.0.0.50:9999"
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">
                API URL reachable from the remote host (auto-detected if empty)
              </p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ssh-local-path">Local Path (NFS Export)</Label>
              <Input
                id="ssh-local-path"
                value={structure.ssh?.local_path || ""}
                onChange={(e) => updateNestedField("ssh", "local_path", e.target.value)}
                placeholder="/Users/dhyanraj/workspace"
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">
                Local NFS export path on this machine
              </p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ssh-mount-path">Mount Path (Remote)</Label>
              <Input
                id="ssh-mount-path"
                value={structure.ssh?.mount_path || ""}
                onChange={(e) => updateNestedField("ssh", "mount_path", e.target.value)}
                placeholder="/mnt/workspace"
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">
                Where the NFS export is mounted on the remote host
              </p>
            </div>
          </CardContent>
        </Card>
        )}

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
