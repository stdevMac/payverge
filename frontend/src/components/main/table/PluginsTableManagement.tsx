"use client";
import React, { useState, useEffect } from "react";
import toast from "react-hot-toast";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Input,
  Textarea,
  Select,
  SelectItem,
  Chip,
  Spinner,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  Image,
} from "@nextui-org/react";
import { PlusIcon } from "@/components/icons/PlusIcon";
import { EditIcon } from "@/components/icons/EditIcon";
import { DeleteIcon } from "@/components/icons/DeleteIcon";
import { SearchIcon } from "@/components/icons/SearchIcon";
import { GridIcon } from "@/components/icons/GridIcon";
import { ListIcon } from "@/components/icons/ListIcon";
import { pluginAPI, type Plugin, type CreatePluginData } from "@/api/plugins";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

const PLUGIN_CATEGORIES = [
  { key: "payment", label: "Payment Processing" },
  { key: "analytics", label: "Analytics & Reporting" },
  { key: "integration", label: "Third-party Integration" },
  { key: "reporting", label: "Tax & Compliance" },
  { key: "marketing", label: "Marketing & CRM" },
];

const TableIcon = ({ size = 24 }: { size?: number }) => (
  <svg
    aria-hidden="true"
    fill="none"
    focusable="false"
    height={size}
    role="presentation"
    viewBox="0 0 24 24"
    width={size}
  >
    <path
      d="M21 7V5a2 2 0 00-2-2H5a2 2 0 00-2 2v2m18 0v12a2 2 0 01-2 2H5a2 2 0 01-2-2V7m18 0H3m14 0v14M7 7v14"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
    />
  </svg>
);

export const PluginsTableManagement = React.memo(function PluginsTableManagement() {
  const { locale } = useSimpleLocale();
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null);

  // View & Filter state
  const [viewMode, setViewMode] = useState<"table" | "list" | "grid">("table");
  const [searchQuery, setSearchQuery] = useState("");
  const [categoryFilter, setCategoryFilter] = useState<string>("all");

  const filteredPlugins = React.useMemo(() => {
    return plugins.filter((plugin) => {
      const matchesSearch =
        plugin.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        plugin.display_name.toLowerCase().includes(searchQuery.toLowerCase());
      const matchesCategory =
        categoryFilter === "all" || plugin.category === categoryFilter;

      return matchesSearch && matchesCategory;
    });
  }, [plugins, searchQuery, categoryFilter]);

  // Form state
  const [formData, setFormData] = useState<CreatePluginData>({
    name: "",
    display_name: "",
    description: "",
    message: "",
    image: "",
    coming_soon: false,
    category: "integration",
    version: "1.0.0",
    features: "[]",
    config_schema: "{}",
  });

  const {
    isOpen: isCreateOpen,
    onOpen: onCreateOpen,
    onClose: onCreateClose,
  } = useDisclosure();
  const {
    isOpen: isEditOpen,
    onOpen: onEditOpen,
    onClose: onEditClose,
  } = useDisclosure();
  const {
    isOpen: isDeleteOpen,
    onOpen: onDeleteOpen,
    onClose: onDeleteClose,
  } = useDisclosure();

  // Load plugins
  const loadPlugins = async () => {
    try {
      setLoading(true);
      const data = await pluginAPI.admin.getAllPlugins();
      setPlugins(data.plugins || []);
    } catch (error) {
      console.error("Failed to load plugins:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadPlugins().catch((err) => console.error("loadPlugins failed:", err));
  }, []);

  // Reset form
  const resetForm = () => {
    setFormData({
      name: "",
      display_name: "",
      description: "",
      message: "",
      image: "",
        coming_soon: false,
      category: "integration",
      version: "1.0.0",
      features: "[]",
      config_schema: "{}",
    });
    setSelectedPlugin(null);
  };

  // Handle create plugin
  const t = (key: string) => {
    const translation = getTranslation(`pluginsManagement.${key}`, locale);
    return typeof translation === "string" ? translation : key;
  };

  const handleCreatePlugin = async () => {
    try {
      await pluginAPI.admin.createPlugin(formData);
      await loadPlugins();
      onCreateClose();
      resetForm();
      toast.success(t("toasts.createSuccess"));
    } catch (error) {
      console.error("Failed to create plugin:", error);
      toast.error(t("toasts.createError"));
    }
  };

  // Handle edit plugin
  const handleEditPlugin = async () => {
    if (!selectedPlugin) return;

    try {
      await pluginAPI.admin.updatePlugin(selectedPlugin.id, formData);
      await loadPlugins();
      onEditClose();
      resetForm();
      toast.success(t("toasts.updateSuccess"));
    } catch (error) {
      console.error("Failed to update plugin:", error);
      toast.error(t("toasts.updateError"));
    }
  };

  // Handle toggle plugin active status
  const handleToggleActive = async (plugin: Plugin) => {
    try {
      await pluginAPI.admin.togglePluginActive(plugin.id);
      await loadPlugins();
      toast.success(
        plugin.is_active
          ? t("toasts.disabledSuccess")
          : t("toasts.enabledSuccess"),
      );
    } catch (error) {
      console.error("Failed to toggle plugin status:", error);
      toast.error(t("toasts.toggleError"));
    }
  };

  // Handle delete plugin
  const handleDeletePlugin = async () => {
    if (!selectedPlugin) return;

    try {
      await pluginAPI.admin.deletePlugin(selectedPlugin.id);
      await loadPlugins();
      onDeleteClose();
      resetForm();
      toast.success(t("toasts.deactivateSuccess"));
    } catch (error) {
      console.error("Failed to delete plugin:", error);
      toast.error(t("toasts.deactivateError"));
    }
  };

  // Open edit modal
  const openEditModal = (plugin: Plugin) => {
    setSelectedPlugin(plugin);
    setFormData({
      name: plugin.name,
      display_name: plugin.display_name,
      description: plugin.description,
      message: plugin.message || plugin.description, // Fallback to description if message is empty
      image: plugin.image,
      coming_soon: plugin.coming_soon,
      category: plugin.category,
      version: plugin.version,
      features: plugin.features,
      config_schema: plugin.config_schema,
    });
    onEditOpen();
  };

  // Open delete modal
  const openDeleteModal = (plugin: Plugin) => {
    setSelectedPlugin(plugin);
    onDeleteOpen();
  };

  if (loading) {
    return (
      <div className="flex justify-center items-center h-64">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t("title")}</h1>
          <p className="text-gray-600">{t("subtitle")}</p>
        </div>
        <Button
          color="primary"
          onPress={onCreateOpen}
          startContent={<PlusIcon />}
        >
          {t("addNewPlugin")}
        </Button>
      </div>

      {/* Filters & View Toggle */}
      <div className="flex flex-col sm:flex-row justify-between gap-4 mb-6">
        <div className="flex gap-4 flex-1">
          <Input
            isClearable
            className="w-full sm:max-w-[44%]"
            placeholder={t("searchPlaceholder")}
            startContent={<SearchIcon />}
            value={searchQuery}
            onClear={() => setSearchQuery("")}
            onValueChange={setSearchQuery}
          />
          <Select
            className="w-full sm:max-w-[200px]"
            placeholder={t("categoryLabel")}
            selectedKeys={[categoryFilter]}
            onChange={(e) => setCategoryFilter(e.target.value || "all")}
          >
            {[
              { key: "all", label: t("allCategories") },
              ...PLUGIN_CATEGORIES.map((cat) => ({
                ...cat,
                label: t(`category.${cat.key}`),
              })),
            ].map((category) => (
              <SelectItem key={category.key} value={category.key}>
                {category.label}
              </SelectItem>
            ))}
          </Select>
        </div>

        <div
          role="group"
          aria-label={t("views.group")}
          className="flex items-center gap-2 bg-default-100 p-1 rounded-lg"
        >
          <Button
            isIconOnly
            size="sm"
            variant={viewMode === "table" ? "solid" : "light"}
            onPress={() => setViewMode("table")}
            aria-label={t("views.table")}
            aria-pressed={viewMode === "table"}
            title={t("views.table")}
          >
            <TableIcon />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant={viewMode === "list" ? "solid" : "light"}
            onPress={() => setViewMode("list")}
            aria-label={t("views.list")}
            aria-pressed={viewMode === "list"}
            title={t("views.list")}
          >
            <ListIcon />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant={viewMode === "grid" ? "solid" : "light"}
            onPress={() => setViewMode("grid")}
            aria-label={t("views.grid")}
            aria-pressed={viewMode === "grid"}
            title={t("views.grid")}
          >
            <GridIcon />
          </Button>
        </div>
      </div>

      {viewMode === "table" ? (
        <Table aria-label="Plugins table">
          <TableHeader>
            <TableColumn>{t("tableHeader.name")}</TableColumn>
            <TableColumn>{t("tableHeader.category")}</TableColumn>
            <TableColumn>{t("tableHeader.status")}</TableColumn>
            <TableColumn>{t("tableHeader.actions")}</TableColumn>
          </TableHeader>
          <TableBody emptyContent={t("noPluginsFound")}>
            {filteredPlugins.map((plugin) => (
              <TableRow key={plugin.id}>
                <TableCell>
                  <div className="flex items-center gap-3">
                    {plugin.image ? (
                      <Image
                        alt={plugin.display_name}
                        className="object-cover rounded-md"
                        height={40}
                        src={plugin.image}
                        width={40}
                      />
                    ) : (
                      <div className="w-10 h-10 rounded-md bg-primary/10 flex items-center justify-center text-primary">
                        <GridIcon size={20} />
                      </div>
                    )}
                    <div>
                      <p className="font-semibold">{plugin.display_name}</p>
                      <p className="text-sm text-gray-500">{plugin.name}</p>
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <Chip size="sm" variant="flat">
                    {t(`category.${plugin.category}`) || plugin.category}
                  </Chip>
                </TableCell>
                <TableCell>
                  <Chip
                    size="sm"
                    color={
                      plugin.coming_soon
                        ? "warning"
                        : plugin.is_active
                          ? "success"
                          : "default"
                    }
                    variant="flat"
                  >
                    {plugin.coming_soon
                      ? t("status.comingSoon")
                      : plugin.is_active
                        ? t("status.active")
                        : t("status.inactive")}
                  </Chip>
                </TableCell>
                <TableCell>
                  <div className="flex gap-2">
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      color={plugin.is_active ? "warning" : "success"}
                      onPress={() => handleToggleActive(plugin)}
                      aria-label={
                        plugin.is_active
                          ? t("actions.deactivate")
                          : t("actions.activate")
                      }
                      title={
                        plugin.is_active
                          ? t("actions.deactivate")
                          : t("actions.activate")
                      }
                    >
                      {plugin.is_active ? "🔒" : "✅"}
                    </Button>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      aria-label={t("actions.edit")}
                      onPress={() => openEditModal(plugin)}
                    >
                      <EditIcon />
                    </Button>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      color="danger"
                      aria-label={t("actions.delete")}
                      onPress={() => openDeleteModal(plugin)}
                    >
                      <DeleteIcon />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : viewMode === "list" ? (
        <div className="flex flex-col gap-3">
          {filteredPlugins.map((plugin) => (
            <Card
              key={plugin.id}
              className="border-none bg-default-50 hover:bg-default-100 transition-colors cursor-pointer"
              isPressable
              onPress={() => openEditModal(plugin)}
            >
              <CardBody className="p-3">
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                  <div className="flex items-center gap-4 flex-1">
                    {plugin.image ? (
                      <Image
                        alt={plugin.display_name}
                        className="object-cover rounded-md"
                        height={48}
                        src={plugin.image}
                        width={48}
                      />
                    ) : (
                      <div className="w-12 h-12 rounded-md bg-primary/10 flex items-center justify-center text-primary">
                        <GridIcon size={24} />
                      </div>
                    )}
                    <div className="flex flex-col">
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-lg">
                          {plugin.display_name}
                        </span>
                        <Chip
                          size="sm"
                          variant="flat"
                          className="text-[10px] h-4"
                        >
                          {plugin.version}
                        </Chip>
                      </div>
                      <div className="flex items-center gap-2 text-default-500 text-sm">
                        <span>{plugin.name}</span>
                        <span>•</span>
                        <span className="capitalize">
                          {t(`category.${plugin.category}`) || plugin.category}
                        </span>
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center gap-6 self-end sm:self-center">

                    <Chip
                      size="sm"
                      color={
                        plugin.coming_soon
                          ? "warning"
                          : plugin.is_active
                            ? "success"
                            : "default"
                      }
                      variant="flat"
                    >
                      {plugin.coming_soon
                        ? t("status.comingSoon")
                        : plugin.is_active
                          ? t("status.active")
                          : t("status.inactive")}
                    </Chip>

                    {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- structural stop-propagation wrapper, not interactive */}
                    <div
                      className="flex gap-1"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        color={plugin.is_active ? "warning" : "success"}
                        onPress={() => handleToggleActive(plugin)}
                        aria-label={
                          plugin.is_active
                            ? t("actions.deactivate")
                            : t("actions.activate")
                        }
                        title={
                          plugin.is_active
                            ? t("actions.deactivate")
                            : t("actions.activate")
                        }
                      >
                        {plugin.is_active ? "🔒" : "✅"}
                      </Button>
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        aria-label={t("actions.edit")}
                        onPress={() => openEditModal(plugin)}
                      >
                        <EditIcon size={18} />
                      </Button>
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        color="danger"
                        aria-label={t("actions.delete")}
                        onPress={() => openDeleteModal(plugin)}
                      >
                        <DeleteIcon size={18} />
                      </Button>
                    </div>
                  </div>
                </div>
              </CardBody>
            </Card>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredPlugins.map((plugin) => (
            <Card key={plugin.id} className="p-2">
              <CardHeader className="justify-between">
                <div className="flex gap-3">
                  {plugin.image ? (
                    <Image
                      alt={plugin.display_name}
                      height={40}
                      radius="sm"
                      src={plugin.image}
                      width={40}
                      className="object-cover"
                    />
                  ) : (
                    <div className="w-10 h-10 rounded-small bg-primary/10 flex items-center justify-center text-primary">
                      <GridIcon size={20} />
                    </div>
                  )}
                  <div className="flex flex-col gap-1 items-start justify-center">
                    <h4 className="text-small font-semibold leading-none text-default-600">
                      {plugin.display_name}
                    </h4>
                    <h5 className="text-small tracking-tight text-default-400">
                      {plugin.name}
                    </h5>
                  </div>
                </div>
                <Button
                  className={
                    plugin.is_active
                      ? "bg-transparent text-default-foreground border-default-200"
                      : ""
                  }
                  color="primary"
                  radius="full"
                  size="sm"
                  variant={plugin.is_active ? "bordered" : "solid"}
                  onPress={() => handleToggleActive(plugin)}
                >
                  {plugin.is_active ? t("status.active") : t("actions.activate")}
                </Button>
              </CardHeader>
              <CardBody className="px-3 py-0 text-small text-default-400">
                <p className="line-clamp-3">
                  {plugin.description || t("noDescription")}
                </p>
                <div className="flex gap-2 mt-2 flex-wrap">
                  <Chip size="sm" variant="flat" className="capitalize">
                    {t(`category.${plugin.category}`) || plugin.category}
                  </Chip>
                  <Chip
                    size="sm"
                    color={
                      plugin.coming_soon
                        ? "warning"
                        : plugin.is_active
                          ? "success"
                          : "default"
                    }
                    variant="flat"
                  >
                    {plugin.coming_soon
                      ? t("status.comingSoon")
                      : plugin.is_active
                        ? t("status.active")
                        : t("status.inactive")}
                  </Chip>
                </div>
              </CardBody>
              <CardFooter className="gap-3">
                <Button
                  fullWidth
                  size="sm"
                  variant="flat"
                  onPress={() => openEditModal(plugin)}
                  startContent={<EditIcon className="w-4 h-4" />}
                >
                  {t("actions.edit")}
                </Button>
                <Button
                  fullWidth
                  size="sm"
                  color="danger"
                  variant="flat"
                  onPress={() => openDeleteModal(plugin)}
                  startContent={<DeleteIcon className="w-4 h-4" />}
                >
                  {t("actions.delete")}
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>
      )}

      {/* Create Plugin Modal */}
      <Modal isOpen={isCreateOpen} onClose={onCreateClose} size="2xl">
        <ModalContent>
          <ModalHeader>{t("createModal.title")}</ModalHeader>
          <ModalBody>
            <div className="grid grid-cols-2 gap-4">
              <Input
                label={t("form.pluginName")}
                placeholder="stripe"
                value={formData.name}
                onChange={(e) =>
                  setFormData({ ...formData, name: e.target.value })
                }
              />
              <Input
                label={t("form.displayName")}
                placeholder="Stripe Integration"
                value={formData.display_name}
                onChange={(e) =>
                  setFormData({ ...formData, display_name: e.target.value })
                }
              />
              <Select
                label={t("form.category")}
                selectedKeys={[formData.category]}
                onSelectionChange={(keys) =>
                  setFormData({
                    ...formData,
                    category: Array.from(keys)[0] as string,
                  })
                }
              >
                {PLUGIN_CATEGORIES.map((category) => (
                  <SelectItem key={category.key} value={category.key}>
                    {t(`category.${category.key}`)}
                  </SelectItem>
                ))}
              </Select>
              <Input
                label={t("form.version")}
                placeholder="1.0.0"
                value={formData.version}
                onChange={(e) =>
                  setFormData({ ...formData, version: e.target.value })
                }
              />
              <Input
                label={t("form.image")}
                placeholder="/images/plugins/plugin.png"
                value={formData.image}
                onChange={(e) =>
                  setFormData({ ...formData, image: e.target.value })
                }
              />
            </div>
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={formData.coming_soon}
                  onChange={(e) =>
                    setFormData({ ...formData, coming_soon: e.target.checked })
                  }
                  className="rounded"
                />
                <span className="text-sm">{t("form.comingSoon")}</span>
              </label>
            </div>
            <Textarea
              label={t("form.description")}
              placeholder="Short plugin description..."
              value={formData.description}
              onChange={(e) =>
                setFormData({ ...formData, description: e.target.value })
              }
              description="Legacy field - use Message field instead"
            />
            <Textarea
              label={t("form.message")}
              placeholder="Enhanced message explaining what users can achieve with this plugin..."
              value={formData.message}
              onChange={(e) =>
                setFormData({ ...formData, message: e.target.value })
              }
              description="This will be automatically translated to Spanish"
              rows={4}
            />
            <Textarea
              label={t("form.features")}
              placeholder='["Feature 1", "Feature 2", "Feature 3"]'
              value={formData.features}
              onChange={(e) =>
                setFormData({ ...formData, features: e.target.value })
              }
            />
            <Textarea
              label={t("form.configSchema")}
              placeholder='{"type": "object", "properties": {"api_key": {"type": "string"}}}'
              value={formData.config_schema}
              onChange={(e) =>
                setFormData({ ...formData, config_schema: e.target.value })
              }
            />
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={onCreateClose}>
              {t("actions.cancel")}
            </Button>
            <Button color="primary" onPress={handleCreatePlugin}>
              {t("actions.create")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Edit Plugin Modal */}
      <Modal isOpen={isEditOpen} onClose={onEditClose} size="2xl">
        <ModalContent>
          <ModalHeader>{t("editModal.title")}</ModalHeader>
          <ModalBody>
            <div className="grid grid-cols-2 gap-4">
              <Input
                label={t("form.pluginName")}
                value={formData.name}
                onChange={(e) =>
                  setFormData({ ...formData, name: e.target.value })
                }
              />
              <Input
                label={t("form.displayName")}
                value={formData.display_name}
                onChange={(e) =>
                  setFormData({ ...formData, display_name: e.target.value })
                }
              />
              <Select
                label={t("form.category")}
                selectedKeys={[formData.category]}
                onSelectionChange={(keys) =>
                  setFormData({
                    ...formData,
                    category: Array.from(keys)[0] as string,
                  })
                }
              >
                {PLUGIN_CATEGORIES.map((category) => (
                  <SelectItem key={category.key} value={category.key}>
                    {t(`category.${category.key}`)}
                  </SelectItem>
                ))}
              </Select>
              <Input
                label={t("form.version")}
                value={formData.version}
                onChange={(e) =>
                  setFormData({ ...formData, version: e.target.value })
                }
              />
              <Input
                label={t("form.image")}
                value={formData.image}
                onChange={(e) =>
                  setFormData({ ...formData, image: e.target.value })
                }
              />
            </div>
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={formData.coming_soon}
                  onChange={(e) =>
                    setFormData({ ...formData, coming_soon: e.target.checked })
                  }
                  className="rounded"
                />
                <span className="text-sm">{t("form.comingSoon")}</span>
              </label>
            </div>
            <Textarea
              label={t("form.description")}
              value={formData.description}
              onChange={(e) =>
                setFormData({ ...formData, description: e.target.value })
              }
              description="Legacy field - use Message field instead"
            />
            <Textarea
              label={t("form.message")}
              placeholder="Enhanced message explaining what users can achieve with this plugin..."
              value={formData.message}
              onChange={(e) =>
                setFormData({ ...formData, message: e.target.value })
              }
              description="This will be automatically translated to Spanish"
              rows={4}
            />
            <Textarea
              label={t("form.features")}
              value={formData.features}
              onChange={(e) =>
                setFormData({ ...formData, features: e.target.value })
              }
            />
            <Textarea
              label={t("form.configSchema")}
              value={formData.config_schema}
              onChange={(e) =>
                setFormData({ ...formData, config_schema: e.target.value })
              }
            />
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={onEditClose}>
              {t("actions.cancel")}
            </Button>
            <Button color="primary" onPress={handleEditPlugin}>
              {t("actions.update")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Delete Plugin Modal */}
      <Modal isOpen={isDeleteOpen} onClose={onDeleteClose}>
        <ModalContent>
          <ModalHeader>{t("deleteModal.title")}</ModalHeader>
          <ModalBody>
            <p>
              {t("deleteModal.confirmMessage").replace(
                "{name}",
                selectedPlugin?.display_name || "",
              )}
            </p>
            <p className="text-sm text-gray-500">{t("deleteModal.warning")}</p>
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={onDeleteClose}>
              {t("actions.cancel")}
            </Button>
            <Button color="danger" onPress={handleDeletePlugin}>
              {t("actions.delete")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
});
