"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FolderOpen,
  Plus,
  Trash2,
  RefreshCw,
  Container,
  Terminal,
  TestTube,
  Folder,
  ChevronUp,
  Home,
  Check,
  Edit,
  X,
  Settings,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Suite,
  BrowseDirectory,
  addSuite,
  deleteSuite,
  syncSuite,
  browseFolders,
  formatRelativeTime,
} from "@/lib/api";
import { TestCaseTree, TestCaseEditor } from "@/components/test-editor";
import { SuiteConfigEditor } from "@/components/suite-editor";

interface SettingsContentProps {
  initialSuites: Suite[];
}

export function SettingsContent({ initialSuites }: SettingsContentProps) {
  const router = useRouter();
  const [suites, setSuites] = useState<Suite[]>(initialSuites);
  const [isAddDialogOpen, setIsAddDialogOpen] = useState(false);
  const [folderPath, setFolderPath] = useState("");
  const [isAdding, setIsAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  // Folder browser state
  const [browsePath, setBrowsePath] = useState("");
  const [browseParent, setBrowseParent] = useState<string | null>(null);
  const [directories, setDirectories] = useState<BrowseDirectory[]>([]);
  const [isSuite, setIsSuite] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [browseError, setBrowseError] = useState<string | null>(null);

  // Test editor state
  const [editingSuiteId, setEditingSuiteId] = useState<number | null>(null);
  const [selectedTest, setSelectedTest] = useState<{
    testId: string;
    testName: string;
  } | null>(null);

  // Config editor state
  const [configSuiteId, setConfigSuiteId] = useState<number | null>(null);

  // Load directory listing when dialog opens or path changes
  useEffect(() => {
    if (isAddDialogOpen) {
      // Try to restore last browsed path from localStorage
      const lastPath = typeof window !== 'undefined'
        ? localStorage.getItem('tsuite-last-browse-path')
        : null;
      loadDirectory(lastPath || undefined);
    }
  }, [isAddDialogOpen]);

  const loadDirectory = async (path?: string) => {
    setIsLoading(true);
    setBrowseError(null);
    try {
      const result = await browseFolders(path);
      setBrowsePath(result.path);
      setBrowseParent(result.parent);
      setDirectories(result.directories);
      setIsSuite(result.is_suite);
      setFolderPath(result.path);
      // Remember last browsed path
      if (typeof window !== 'undefined') {
        localStorage.setItem('tsuite-last-browse-path', result.path);
      }
    } catch (err) {
      setBrowseError(err instanceof Error ? err.message : "Failed to load directory");
    } finally {
      setIsLoading(false);
    }
  };

  const handleNavigate = (path: string) => {
    loadDirectory(path);
  };

  const handleAddSuite = async () => {
    if (!folderPath.trim()) return;

    setIsAdding(true);
    setAddError(null);

    try {
      const newSuite = await addSuite(folderPath.trim());
      setSuites([...suites, newSuite]);
      setFolderPath("");
      setIsAddDialogOpen(false);
      router.refresh();
    } catch (err) {
      setAddError(err instanceof Error ? err.message : "Failed to add suite");
    } finally {
      setIsAdding(false);
    }
  };

  const handleDeleteSuite = async (suiteId: number) => {
    setDeletingId(suiteId);
    try {
      await deleteSuite(suiteId);
      setSuites(suites.filter((s) => s.id !== suiteId));
      router.refresh();
    } catch (err) {
      console.error("Failed to delete suite:", err);
    } finally {
      setDeletingId(null);
    }
  };

  const handleSyncSuite = async (suiteId: number) => {
    setSyncingId(suiteId);
    try {
      const updatedSuite = await syncSuite(suiteId);
      setSuites(suites.map((s) => (s.id === suiteId ? updatedSuite : s)));
      router.refresh();
    } catch (err) {
      console.error("Failed to sync suite:", err);
    } finally {
      setSyncingId(null);
    }
  };

  const handleDialogOpenChange = (open: boolean) => {
    setIsAddDialogOpen(open);
    if (!open) {
      // Reset state when closing
      setFolderPath("");
      setAddError(null);
      setBrowseError(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Add Suite Section */}
      <Card className="rounded-md">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
          <CardTitle className="text-lg font-medium">Test Suites</CardTitle>
          <Dialog open={isAddDialogOpen} onOpenChange={handleDialogOpenChange}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-2">
                <Plus className="h-4 w-4" />
                Add Suite
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-2xl">
              <DialogHeader>
                <DialogTitle>Add Test Suite</DialogTitle>
                <DialogDescription>
                  Browse to select a folder containing a valid test suite with
                  config.yaml and suites/ directory
                </DialogDescription>
              </DialogHeader>

              {/* Path input */}
              <div className="grid gap-2">
                <Label htmlFor="folder-path">Folder Path</Label>
                <div className="flex gap-2">
                  <Input
                    id="folder-path"
                    placeholder="/path/to/test-suite"
                    value={folderPath}
                    onChange={(e) => setFolderPath(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        if (isSuite) {
                          handleAddSuite();
                        } else {
                          loadDirectory(folderPath);
                        }
                      }
                    }}
                  />
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={() => loadDirectory(folderPath)}
                    title="Go to path"
                  >
                    <FolderOpen className="h-4 w-4" />
                  </Button>
                </div>
              </div>

              {/* Folder Browser */}
              <div className="border rounded-md">
                {/* Browser toolbar */}
                <div className="flex items-center gap-2 p-2 border-b bg-muted/30">
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => loadDirectory()}
                    title="Home"
                    className="h-8 w-8"
                  >
                    <Home className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => browseParent && loadDirectory(browseParent)}
                    disabled={!browseParent}
                    title="Parent directory"
                    className="h-8 w-8"
                  >
                    <ChevronUp className="h-4 w-4" />
                  </Button>
                  <span className="text-sm text-muted-foreground truncate flex-1">
                    {browsePath}
                  </span>
                  {isSuite && (
                    <Badge className="bg-green-500/20 text-green-500 border-green-500/50">
                      <Check className="h-3 w-3 mr-1" />
                      Valid Suite
                    </Badge>
                  )}
                </div>

                {/* Directory listing */}
                <ScrollArea className="h-64">
                  {isLoading ? (
                    <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
                      Loading...
                    </div>
                  ) : browseError ? (
                    <div className="flex items-center justify-center h-full text-sm text-destructive p-4">
                      {browseError}
                    </div>
                  ) : directories.length === 0 ? (
                    <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
                      No subdirectories
                    </div>
                  ) : (
                    <div className="p-2">
                      {directories.map((dir) => (
                        <button
                          key={dir.path}
                          onClick={() => handleNavigate(dir.path)}
                          className="w-full flex items-center gap-2 px-3 py-2 text-left rounded-md hover:bg-muted/50 transition-colors"
                        >
                          <Folder
                            className={`h-4 w-4 ${
                              dir.is_suite ? "text-green-500" : "text-primary"
                            }`}
                          />
                          <span className="flex-1 truncate text-sm">
                            {dir.name}
                          </span>
                          {dir.is_suite && (
                            <Badge
                              variant="outline"
                              className="text-[10px] border-green-500/50 text-green-500"
                            >
                              suite
                            </Badge>
                          )}
                        </button>
                      ))}
                    </div>
                  )}
                </ScrollArea>
              </div>

              {/* Error message */}
              {addError && (
                <p className="text-sm text-destructive">{addError}</p>
              )}

              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => handleDialogOpenChange(false)}
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleAddSuite}
                  disabled={isAdding || !isSuite}
                >
                  {isAdding ? "Adding..." : "Add Suite"}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </CardHeader>
        <CardContent>
          {suites.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <FolderOpen className="h-12 w-12 text-muted-foreground/50" />
              <h3 className="mt-4 text-lg font-medium">No test suites</h3>
              <p className="mt-2 text-sm text-muted-foreground">
                Add a test suite folder to get started
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {suites.map((suite, idx) => (
                <Card
                  key={suite.id || `suite-${idx}`}
                  className="rounded-md bg-muted/30 hover:bg-muted/50 transition-colors"
                >
                  <CardContent className="px-4 py-3">
                    <div className="flex items-center justify-between">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <h4 className="font-medium truncate">
                            {suite.suite_name}
                          </h4>
                          <Badge
                            variant="outline"
                            className={
                              suite.mode === "docker"
                                ? "border-blue-500/50 text-blue-500"
                                : "border-orange-500/50 text-orange-500"
                            }
                          >
                            {suite.mode === "docker" ? (
                              <Container className="h-3 w-3 mr-1" />
                            ) : (
                              <Terminal className="h-3 w-3 mr-1" />
                            )}
                            {suite.mode}
                          </Badge>
                        </div>
                        <p className="text-sm text-muted-foreground truncate mt-1">
                          {suite.folder_path}
                        </p>
                        <div className="flex items-center gap-4 mt-2 text-xs text-muted-foreground">
                          <span className="flex items-center gap-1">
                            <TestTube className="h-3 w-3" />
                            {suite.test_count} tests
                          </span>
                          {suite.last_synced_at && (
                            <span>
                              Synced {formatRelativeTime(suite.last_synced_at)}
                            </span>
                          )}
                        </div>
                      </div>
                      <div className="flex items-center gap-2 ml-4">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => {
                            setConfigSuiteId(suite.id);
                          }}
                          title="Edit config"
                        >
                          <Settings className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => {
                            setEditingSuiteId(suite.id);
                            setSelectedTest(null);
                          }}
                          title="Edit tests"
                        >
                          <Edit className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleSyncSuite(suite.id)}
                          disabled={syncingId === suite.id}
                          title="Sync from YAML"
                        >
                          <RefreshCw
                            className={`h-4 w-4 ${
                              syncingId === suite.id ? "animate-spin" : ""
                            }`}
                          />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleDeleteSuite(suite.id)}
                          disabled={deletingId === suite.id}
                          className="text-destructive hover:text-destructive"
                          title="Remove suite"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Config Editor Section */}
      {configSuiteId && (
        <Card className="rounded-md">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
            <CardTitle className="text-lg font-medium">
              Edit Config -{" "}
              {suites.find((s) => s.id === configSuiteId)?.suite_name}
            </CardTitle>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setConfigSuiteId(null)}
            >
              <X className="h-4 w-4" />
            </Button>
          </CardHeader>
          <CardContent>
            <div className="border rounded-md overflow-hidden h-[600px]">
              <SuiteConfigEditor
                suiteId={configSuiteId}
                suiteName={
                  suites.find((s) => s.id === configSuiteId)?.suite_name || ""
                }
              />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Test Case Editor Section */}
      {editingSuiteId && (
        <Card className="rounded-md">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
            <CardTitle className="text-lg font-medium">
              Edit Test Cases -{" "}
              {suites.find((s) => s.id === editingSuiteId)?.suite_name}
            </CardTitle>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => {
                setEditingSuiteId(null);
                setSelectedTest(null);
              }}
            >
              <X className="h-4 w-4" />
            </Button>
          </CardHeader>
          <CardContent>
            <div className="flex gap-4 h-[600px]">
              {/* Tree navigation */}
              <div className="w-72 border rounded-md overflow-hidden flex flex-col">
                <div className="p-2 border-b bg-muted/30">
                  <span className="text-sm font-medium">Test Cases</span>
                </div>
                <ScrollArea className="flex-1 h-0">
                  <div className="p-2">
                    <TestCaseTree
                      suiteId={editingSuiteId}
                      selectedTestId={selectedTest?.testId}
                      onSelectTest={(testId, testName) =>
                        setSelectedTest({ testId, testName })
                      }
                    />
                  </div>
                </ScrollArea>
              </div>

              {/* Editor panel */}
              <div className="flex-1 border rounded-md overflow-hidden">
                {selectedTest ? (
                  <TestCaseEditor
                    suiteId={editingSuiteId}
                    testId={selectedTest.testId}
                    testName={selectedTest.testName}
                  />
                ) : (
                  <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
                    <Edit className="h-12 w-12 opacity-50 mb-4" />
                    <p className="text-sm">Select a test case to edit</p>
                  </div>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Info Section */}
      <Card className="rounded-md">
        <CardHeader>
          <CardTitle className="text-lg font-medium">Suite Modes</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-start gap-3">
            <Badge
              variant="outline"
              className="border-blue-500/50 text-blue-500 mt-0.5"
            >
              <Container className="h-3 w-3 mr-1" />
              docker
            </Badge>
            <div>
              <p className="text-sm">
                Tests run inside a Docker container with isolated environment
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                Requires Docker to be running. Best for integration tests.
              </p>
            </div>
          </div>
          <div className="flex items-start gap-3">
            <Badge
              variant="outline"
              className="border-orange-500/50 text-orange-500 mt-0.5"
            >
              <Terminal className="h-3 w-3 mr-1" />
              standalone
            </Badge>
            <div>
              <p className="text-sm">
                Tests run directly on the host machine without Docker
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                Faster execution. Best for unit tests and library tests.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
