// Docker Manager JavaScript Application
class DockerManager {
    constructor() {
        this.ws = null;
        this.currentTab = 'dashboard';
        this.eventCount = 0;
        this.refreshInterval = null;
        this.lastRefreshTime = 0;
        this.refreshCooldown = 5000; // 5 seconds cooldown between refreshes
        this.forceRefreshInterval = 120000; // Force refresh every 2 minutes
        this.hostInfoInterval = null;
        this.init();
    }

    init() {
        this.setupEventListeners();
        this.loadInitialData();
        this.startPeriodicRefresh();
        this.startHostInfoRefresh();
        // Set initial connection status for dashboard
        this.updateConnectionStatus(false);
    }

    setupEventListeners() {
        // Navigation
        document.querySelectorAll('.nav-item').forEach(item => {
            item.addEventListener('click', (e) => {
                e.preventDefault();
                const tab = item.dataset.tab;
                this.switchTab(tab);
            });
        });

        // Modal controls
        document.querySelector('.modal-close')?.addEventListener('click', () => {
            this.closeModal();
        });

        // Click outside modal to close
        document.getElementById('container-modal')?.addEventListener('click', (e) => {
            if (e.target.id === 'container-modal') {
                this.closeModal();
            }
        });

        // Clear events button
        document.getElementById('clear-events')?.addEventListener('click', () => {
            this.clearEvents();
        });

        // Container detail tabs
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.switchDetailTab(btn.dataset.tab);
            });
        });
    }

    setupWebSocket() {
        // Only setup WebSocket if not already connected and on events tab
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            return;
        }

        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${location.host}/ws`;

        try {
            this.ws = new WebSocket(wsUrl);

            this.ws.onopen = () => {
                console.log('WebSocket connected');
                this.updateConnectionStatus(true);
            };

            this.ws.onmessage = (event) => {
                const eventData = JSON.parse(event.data);
                this.handleDockerEvent(eventData);
            };

            this.ws.onclose = () => {
                console.log('WebSocket disconnected');
                this.updateConnectionStatus(false);
                // Only attempt to reconnect if still on events tab
                if (this.currentTab === 'events') {
                    setTimeout(() => this.setupWebSocket(), 5000);
                }
            };

            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.updateConnectionStatus(false);
            };
        } catch (error) {
            console.error('Failed to create WebSocket:', error);
            this.updateConnectionStatus(false);
        }
    }

    closeWebSocket() {
        if (this.ws) {
            console.log('Closing WebSocket connection');
            // Remove event listeners to prevent reconnection attempts
            this.ws.onopen = null;
            this.ws.onmessage = null;
            this.ws.onerror = null;
            this.ws.onclose = null;

            // Close the connection
            if (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING) {
                this.ws.close();
            }
            this.ws = null;
            this.updateConnectionStatus(false);
        }
    }

    updateConnectionStatus(connected) {
        const statusElement = document.getElementById('connection-status');
        if (statusElement) {
            const icon = statusElement.querySelector('i');
            const text = statusElement.querySelector('span');

            if (this.currentTab === 'events') {
                if (connected) {
                    icon.className = 'fas fa-circle';
                    text.textContent = 'Events Connected';
                    statusElement.style.color = '#059669';
                } else {
                    icon.className = 'fas fa-circle';
                    text.textContent = 'Events Disconnected';
                    statusElement.style.color = '#dc2626';
                }
            } else {
                icon.className = 'fas fa-wifi';
                text.textContent = 'API Ready';
                statusElement.style.color = '#059669';
            }
        }
    }

    handleDockerEvent(eventData) {
        this.eventCount++;
        const eventsCountElement = document.getElementById('events-count');
        if (eventsCountElement) {
            eventsCountElement.textContent = `${this.eventCount} events`;
        }

        // Add to events stream if on events tab
        if (this.currentTab === 'events') {
            this.addEventToStream(eventData);
        }

        // Smart refresh with cooldown to avoid excessive API calls (only if not on events tab)
        if (this.currentTab !== 'events' && ['container', 'image', 'network', 'volume'].includes(eventData.Type)) {
            this.smartRefresh(eventData);
        }
    }

    // Smart refresh with cooldown and selective updating
    smartRefresh(eventData) {
        const now = Date.now();

        // Apply cooldown to prevent excessive API calls
        if (now - this.lastRefreshTime < this.refreshCooldown) {
            console.log('Refresh skipped due to cooldown');
            return;
        }

        // Update specific data based on event type
        switch (eventData.Type) {
            case 'container':
                this.handleContainerEvent(eventData);
                break;
            case 'image':
                if (this.currentTab === 'images') {
                    this.loadImages();
                }
                // Only refresh dashboard stats if significant image events
                if (['delete', 'untag'].includes(eventData.Action)) {
                    this.refreshDashboardStats();
                }
                break;
            case 'network':
                if (this.currentTab === 'networks') {
                    this.loadNetworks();
                }
                break;
            case 'volume':
                if (this.currentTab === 'volumes') {
                    this.loadVolumes();
                }
                break;
        }

        this.lastRefreshTime = now;
    }

    // Handle container events more intelligently
    handleContainerEvent(eventData) {
        const action = eventData.Action;


        // Refresh dashboard stats for significant container events
        if (['start', 'stop', 'die', 'destroy', 'create'].includes(action)) {
            // Always refresh containers tab if it's active
            if (this.currentTab === 'containers') {
                this.loadContainers();
            }
            this.refreshDashboardStats();
        }

        // For state changes, we can be smarter and just update the specific container
        // if we want to implement per-container updates in the future
    }

    // Refresh only dashboard stats, not full info
    async refreshDashboardStats() {
        if (this.currentTab === 'dashboard') {
            try {
                const statsResponse = await fetch('/api/system/stats');
                const stats = await statsResponse.json();
                this.updateDashboardStats(stats);
            } catch (error) {
                console.error('Error refreshing dashboard stats:', error);
            }
        }
    }

    addEventToStream(eventData) {
        const eventsStream = document.getElementById('events-stream');
        if (!eventsStream) return;

        const eventElement = document.createElement('div');
        eventElement.className = 'event-item';

        const timestamp = new Date(eventData.time * 1000).toLocaleString();
        eventElement.innerHTML = `
            <div class="event-time">${timestamp}</div>
            <div class="event-type">${eventData.Type}: ${eventData.Action}</div>
            <div class="event-details">${this.formatEventDetails(eventData)}</div>
        `;

        eventsStream.insertBefore(eventElement, eventsStream.firstChild);

        // Keep only last 100 events
        while (eventsStream.children.length > 100) {
            eventsStream.removeChild(eventsStream.lastChild);
        }
    }



    formatEventDetails(eventData) {
        if (eventData.Actor && eventData.Actor.Attributes) {
            const attrs = eventData.Actor.Attributes;
            if (attrs.name) {
                return `Name: ${attrs.name}`;
            }
            if (attrs.image) {
                return `Image: ${attrs.image}`;
            }
        }
        return eventData.Actor ? eventData.Actor.ID?.substring(0, 12) : '';
    }

    clearEvents() {
        const eventsStream = document.getElementById('events-stream');
        if (eventsStream) {
            eventsStream.innerHTML = '<div class="loading">Events cleared</div>';
        }
        this.eventCount = 0;
        document.getElementById('events-count').textContent = '0 events';
    }

    switchTab(tabName) {
        // Update navigation
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.remove('active');
        });
        document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');

        // Update content
        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.remove('active');
        });
        document.getElementById(tabName).classList.add('active');

        this.currentTab = tabName;

        // Handle WebSocket connection based on tab
        if (tabName === 'events') {
            this.setupWebSocket();
        } else {
            this.closeWebSocket();
        }

        // Update connection status display based on current tab
        this.updateConnectionStatus(this.ws && this.ws.readyState === WebSocket.OPEN);

        this.loadTabData(tabName);
    }

    switchDetailTab(tabName) {
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.classList.remove('active');
        });
        document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');

        document.querySelectorAll('.detail-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        document.getElementById(`container-${tabName}`).classList.add('active');
    }

    async loadInitialData() {
        await this.loadDashboardData();
    }

    async loadTabData(tabName) {
        switch (tabName) {
            case 'dashboard':
                await this.loadDashboardData();
                break;
            case 'containers':
                await this.loadContainers();
                break;
            case 'images':
                await this.loadImages();
                break;
            case 'networks':
                await this.loadNetworks();
                break;
            case 'volumes':
                await this.loadVolumes();
                break;
            case 'system':
                await this.loadSystemInfo();
                break;
            case 'services':
                await this.loadServices();
                break;
            case 'events':
                this.loadEvents();
                break;
        }
    }

    refreshCurrentTabData() {
        this.loadTabData(this.currentTab);
    }

    startPeriodicRefresh() {
        // Reduced frequency: refresh every 2 minutes as fallback
        // Most updates should come from WebSocket events
        this.refreshInterval = setInterval(() => {
            if (this.currentTab !== 'events') {
                console.log('Force refresh (fallback)');
                this.refreshCurrentTabData();
            }
        }, this.forceRefreshInterval);
    }

    async loadDashboardData() {
        try {
            const [statsResponse, infoResponse, hostResponse] = await Promise.all([
                fetch('/api/system/stats'),
                fetch('/api/info'),
                fetch('/api/system/host')
            ]);

            const stats = await statsResponse.json();
            const info = await infoResponse.json();
            const hostInfo = await hostResponse.json();

            this.updateDashboardStats(stats);
            this.updateSystemInfo(info);
            this.updateHostSystemInfo(hostInfo);
        } catch (error) {
            console.error('Error loading dashboard data:', error);
        }
    }

    startHostInfoRefresh() {
        // Refresh host system info every 30 seconds
        this.hostInfoInterval = setInterval(() => {
            if (this.currentTab === 'dashboard') {
                this.loadHostSystemInfo();
            }
        }, 30000);
    }

    async loadHostSystemInfo() {
        try {
            const response = await fetch('/api/system/host');
            const hostInfo = await response.json();
            this.updateHostSystemInfo(hostInfo);
        } catch (error) {
            console.error('Error loading host system info:', error);
        }
    }

    updateHostSystemInfo(hostInfo) {
        document.getElementById('host-uptime').textContent = hostInfo.uptime || 'N/A';

        const loadText = `${hostInfo.load_avg_1?.toFixed(2) || 'N/A'} / ${hostInfo.load_avg_5?.toFixed(2) || 'N/A'} / ${hostInfo.load_avg_15?.toFixed(2) || 'N/A'}`;
        document.getElementById('host-load').textContent = loadText;

        const memoryUsedGB = (hostInfo.memory_used / (1024 * 1024 * 1024)).toFixed(1);
        const memoryTotalGB = (hostInfo.memory_total / (1024 * 1024 * 1024)).toFixed(1);
        const memoryText = `${memoryUsedGB} GB / ${memoryTotalGB} GB (${hostInfo.memory_used_percent?.toFixed(1) || 'N/A'}%)`;
        document.getElementById('host-memory').textContent = memoryText;

        document.getElementById('host-connections').textContent = hostInfo.network_connections || 'N/A';
        document.getElementById('host-cpu-cores').textContent = hostInfo.cpu_cores || 'N/A';
    }

    updateDashboardStats(stats) {
        document.getElementById('containers-total').textContent = stats.containers.total;
        document.getElementById('containers-running').textContent = `${stats.containers.running} Running`;
        document.getElementById('containers-stopped').textContent = `${stats.containers.stopped} Stopped`;

        document.getElementById('images-total').textContent = stats.images.total;
        document.getElementById('images-size').textContent = this.formatBytes(stats.images.size);

        document.getElementById('networks-total').textContent = stats.networks.total;
        document.getElementById('volumes-total').textContent = stats.volumes.total;
    }

    updateSystemInfo(info) {
        if (info.version) {
            document.getElementById('docker-version').textContent = info.version.Version;
            document.getElementById('system-arch').textContent = info.version.Arch;
            document.getElementById('system-os').textContent = info.version.Os;
        }

        if (info.system_info) {
            document.getElementById('system-memory').textContent = this.formatBytes(info.system_info.MemTotal);
            document.getElementById('system-cpus').textContent = info.system_info.NCPU;

            // Update system status in header
            const systemStatus = document.getElementById('system-status');
            if (systemStatus) {
                const containerCount = info.containers ? info.containers.length : 0;
                const runningCount = info.containers ? info.containers.filter(c => c.State === 'running').length : 0;
                systemStatus.textContent = `${runningCount}/${containerCount} containers running`;
            }
        }
    }

    async loadContainers() {
        try {
            const response = await fetch('/api/containers');
            const containers = await response.json();
            this.renderContainersTable(containers);
        } catch (error) {
            console.error('Error loading containers:', error);
        }
    }

    renderContainersTable(containers) {
        const tbody = document.querySelector('#containers-table tbody');
        if (!tbody) return;

        if (containers.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center">No containers found</td></tr>';
            return;
        }

        // Always group by Docker Compose
        this.renderGroupedContainers(containers, tbody);
    }

    renderGroupedContainers(containers, tbody) {
        const groups = this.groupContainersByCompose(containers);
        let html = '';

        // Sort groups: Compose projects first, then standalone
        const sortedGroupNames = Object.keys(groups).sort((a, b) => {
            if (a === 'standalone' && b !== 'standalone') return 1;
            if (a !== 'standalone' && b === 'standalone') return -1;
            return a.localeCompare(b);
        });

        sortedGroupNames.forEach(groupName => {
            const groupContainers = groups[groupName];

            // Group header
            html += `
                <tr class="group-header">
                    <td colspan="6">
                        <strong>${this.getComposeGroupDisplayName(groupName)} (${groupContainers.length})</strong>
                    </td>
                </tr>
            `;

            // Group containers
            groupContainers.forEach(container => {
                html += this.renderContainerRow(container);
            });
        });

        tbody.innerHTML = html;
    }

    groupContainersByCompose(containers) {
        const groups = {};

        containers.forEach(container => {
            let groupKey = 'standalone';

            // Group by docker-compose project (com.docker.compose.project label)
            if (container.Labels && container.Labels['com.docker.compose.project']) {
                groupKey = container.Labels['com.docker.compose.project'];
            }

            if (!groups[groupKey]) {
                groups[groupKey] = [];
            }
            groups[groupKey].push(container);
        });

        return groups;
    }

    getComposeGroupDisplayName(groupName) {
        if (groupName === 'standalone') {
            return '🐳 Standalone Containers';
        } else {
            return `🐙 ${groupName}`;
        }
    }

    renderContainerRow(container) {
        const name = container.Names[0].substring(1); // Remove leading /
        const image = container.Image;
        const status = this.getContainerStatus(container);
        const ports = this.formatPorts(container.Ports);
        const created = new Date(container.Created * 1000).toLocaleString();

        return `
            <tr>
                <td>${name}</td>
                <td><code>${image}</code></td>
                <td><span class="status-badge ${status.class}">${status.text}</span></td>
                <td>${ports}</td>
                <td>${created}</td>
                <td class="actions">
                    ${this.getContainerActions(container)}
                </td>
            </tr>
        `;
    }

    getContainerStatus(container) {
        const state = container.State.toLowerCase();
        switch (state) {
            case 'running':
                return { class: 'status-running', text: 'Running' };
            case 'paused':
                return { class: 'status-paused', text: 'Paused' };
            case 'exited':
                return { class: 'status-stopped', text: 'Exited' };
            case 'dead':
                return { class: 'status-error', text: 'Dead' };
            case 'restarting':
                return { class: 'status-warning', text: 'Restarting' };
            case 'created':
                return { class: 'status-created', text: 'Created' };
            default:
                return { class: 'status-stopped', text: 'Stopped' };
        }
    }

    formatPorts(ports) {
        if (!ports || ports.length === 0) return '<span class="text-muted">-</span>';

        return ports.map(port => {
            if (port.PublicPort && port.IP) {
                // Show full binding info: IP:PublicPort→PrivatePort/Protocol
                const ip = port.IP === '0.0.0.0' ? '*' : port.IP;
                const protocol = port.Type || 'tcp';
                return `<span class="port-mapping" title="Host ${ip}:${port.PublicPort} → Container ${port.PrivatePort}/${protocol}">
                    <i class="fas fa-external-link-alt"></i> ${ip}:${port.PublicPort}→${port.PrivatePort}
                </span>`;
            } else if (port.PublicPort) {
                // Host port without IP specified
                const protocol = port.Type || 'tcp';
                return `<span class="port-mapping" title="Host port ${port.PublicPort} → Container ${port.PrivatePort}/${protocol}">
                    <i class="fas fa-external-link-alt"></i> ${port.PublicPort}→${port.PrivatePort}
                </span>`;
            } else {
                // Internal port only (not exposed)
                const protocol = port.Type || 'tcp';
                return `<span class="port-internal" title="Internal port ${port.PrivatePort}/${protocol}">
                    <i class="fas fa-lock"></i> ${port.PrivatePort}
                </span>`;
            }
        }).join('<br>');
    }

    getContainerActions(container) {
        const id = container.Id;
        const isRunning = container.State === 'running';

        let actions = `<button class="btn btn-info" onclick="dockerManager.showContainerDetail('${id}')">
            <i class="fas fa-info-circle"></i> Details
        </button>`;

        if (isRunning) {
            actions += `<button class="btn btn-danger" onclick="dockerManager.stopContainer('${id}')">
                <i class="fas fa-stop"></i> Stop
            </button>`;
            actions += `<button class="btn btn-secondary" onclick="dockerManager.restartContainer('${id}')">
                <i class="fas fa-redo"></i> Restart
            </button>`;
        } else {
            actions += `<button class="btn btn-success" onclick="dockerManager.startContainer('${id}')">
                <i class="fas fa-play"></i> Start
            </button>`;
        }

        return actions;
    }

    async loadImages() {
        try {
            const response = await fetch('/api/images');
            const images = await response.json();
            this.renderImagesTable(images);
        } catch (error) {
            console.error('Error loading images:', error);
        }
    }

    renderImagesTable(images) {
        const tbody = document.querySelector('#images-table tbody');
        if (!tbody) return;

        if (images.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="text-center">No images found</td></tr>';
            return;
        }

        tbody.innerHTML = images.map(image => {
            const repo = image.RepoTags && image.RepoTags.length > 0 ?
                image.RepoTags[0].split(':')[0] : '<none>';
            const tag = image.RepoTags && image.RepoTags.length > 0 ?
                image.RepoTags[0].split(':')[1] : '<none>';
            const id = image.Id.substring(7, 19); // Remove sha256: prefix and truncate
            const size = this.formatBytes(image.Size);
            const created = new Date(image.Created * 1000).toLocaleString();

            return `
                <tr>
                    <td>${repo}</td>
                    <td><code>${tag}</code></td>
                    <td><code>${id}</code></td>
                    <td>${size}</td>
                    <td>${created}</td>
                </tr>
            `;
        }).join('');
    }

    async loadNetworks() {
        try {
            const response = await fetch('/api/networks');
            const networks = await response.json();
            this.renderNetworksTable(networks);
        } catch (error) {
            console.error('Error loading networks:', error);
        }
    }

    renderNetworksTable(networks) {
        const tbody = document.querySelector('#networks-table tbody');
        if (!tbody) return;

        if (networks.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="text-center">No networks found</td></tr>';
            return;
        }

        tbody.innerHTML = networks.map(network => {
            const name = network.Name;
            const driver = network.Driver;
            const scope = network.Scope;
            const created = new Date(network.Created).toLocaleString();
            const subnet = this.getNetworkSubnet(network);

            return `
                <tr>
                    <td>${name}</td>
                    <td>${driver}</td>
                    <td>${scope}</td>
                    <td>${created}</td>
                    <td><code>${subnet}</code></td>
                </tr>
            `;
        }).join('');
    }

    getNetworkSubnet(network) {
        if (network.IPAM && network.IPAM.Config && network.IPAM.Config.length > 0) {
            return network.IPAM.Config[0].Subnet || '-';
        }
        return '-';
    }

    async loadVolumes() {
        try {
            const response = await fetch('/api/volumes');
            const volumeData = await response.json();
            this.renderVolumesTable(volumeData.Volumes || []);
        } catch (error) {
            console.error('Error loading volumes:', error);
        }
    }

    renderVolumesTable(volumes) {
        const tbody = document.querySelector('#volumes-table tbody');
        if (!tbody) return;

        if (volumes.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center">No volumes found</td></tr>';
            return;
        }

        tbody.innerHTML = volumes.map(volume => {
            const name = volume.Name;
            const driver = volume.Driver;
            const mountpoint = volume.Mountpoint;
            const created = new Date(volume.CreatedAt).toLocaleString();

            return `
                <tr>
                    <td>${name}</td>
                    <td>${driver}</td>
                    <td><code class="truncate">${mountpoint}</code></td>
                    <td>${created}</td>
                </tr>
            `;
        }).join('');
    }

    async loadSystemInfo() {
        try {
            const response = await fetch('/api/info');
            const info = await response.json();
            this.renderSystemInfo(info);
        } catch (error) {
            console.error('Error loading system info:', error);
        }
    }

    renderSystemInfo(info) {
        const dockerInfoDiv = document.getElementById('docker-info');
        const diskUsageDiv = document.getElementById('disk-usage');

        if (dockerInfoDiv && info.system_info) {
            dockerInfoDiv.innerHTML = this.formatSystemInfo(info.system_info, info.version);
        }

        if (diskUsageDiv && info.disk_usage) {
            diskUsageDiv.innerHTML = this.formatDiskUsage(info.disk_usage);
        }
    }

    formatSystemInfo(sysInfo, version) {
        return `
            <div class="info-row">
                <span class="label">Docker Version:</span>
                <span class="value">${version.Version}</span>
            </div>
            <div class="info-row">
                <span class="label">API Version:</span>
                <span class="value">${version.ApiVersion}</span>
            </div>
            <div class="info-row">
                <span class="label">Architecture:</span>
                <span class="value">${sysInfo.Architecture}</span>
            </div>
            <div class="info-row">
                <span class="label">Operating System:</span>
                <span class="value">${sysInfo.OperatingSystem}</span>
            </div>
            <div class="info-row">
                <span class="label">Kernel Version:</span>
                <span class="value">${sysInfo.KernelVersion}</span>
            </div>
            <div class="info-row">
                <span class="label">Total Memory:</span>
                <span class="value">${this.formatBytes(sysInfo.MemTotal)}</span>
            </div>
            <div class="info-row">
                <span class="label">CPUs:</span>
                <span class="value">${sysInfo.NCPU}</span>
            </div>
            <div class="info-row">
                <span class="label">Server Version:</span>
                <span class="value">${sysInfo.ServerVersion}</span>
            </div>
            <div class="info-row">
                <span class="label">Storage Driver:</span>
                <span class="value">${sysInfo.Driver}</span>
            </div>
            <div class="info-row">
                <span class="label">Logging Driver:</span>
                <span class="value">${sysInfo.LoggingDriver}</span>
            </div>
        `;
    }

    formatDiskUsage(diskUsage) {
        const containerSize = diskUsage.LayersSize || 0;
        const imageSize = diskUsage.Images?.reduce((total, img) => total + (img.Size || 0), 0) || 0;
        const volumeSize = diskUsage.Volumes?.reduce((total, vol) => total + (vol.Size || 0), 0) || 0;

        return `
            <div class="info-row">
                <span class="label">Images:</span>
                <span class="value">${this.formatBytes(imageSize)}</span>
            </div>
            <div class="info-row">
                <span class="label">Containers:</span>
                <span class="value">${this.formatBytes(containerSize)}</span>
            </div>
            <div class="info-row">
                <span class="label">Volumes:</span>
                <span class="value">${this.formatBytes(volumeSize)}</span>
            </div>
            <div class="info-row">
                <span class="label">Total:</span>
                <span class="value">${this.formatBytes(imageSize + containerSize + volumeSize)}</span>
            </div>
        `;
    }

    loadEvents() {
        const eventsStream = document.getElementById('events-stream');
        if (eventsStream && eventsStream.children.length <= 1) {
            eventsStream.innerHTML = '<div class="loading">Waiting for events...</div>';
        }
    }

    // Container Actions
    async startContainer(containerId) {
        try {
            const response = await fetch(`/api/containers/${containerId}/start`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification('Container started successfully', 'success');
                this.loadContainers();
            } else {
                throw new Error('Failed to start container');
            }
        } catch (error) {
            this.showNotification('Error starting container: ' + error.message, 'error');
        }
    }

    async stopContainer(containerId) {
        try {
            const response = await fetch(`/api/containers/${containerId}/stop`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification('Container stopped successfully', 'success');
                this.loadContainers();
            } else {
                throw new Error('Failed to stop container');
            }
        } catch (error) {
            this.showNotification('Error stopping container: ' + error.message, 'error');
        }
    }

    async restartContainer(containerId) {
        try {
            const response = await fetch(`/api/containers/${containerId}/restart`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification('Container restarted successfully', 'success');
                this.loadContainers();
            } else {
                throw new Error('Failed to restart container');
            }
        } catch (error) {
            this.showNotification('Error restarting container: ' + error.message, 'error');
        }
    }

    async showContainerDetail(containerId) {
        try {
            const response = await fetch(`/api/containers/${containerId}`);
            const containerDetail = await response.json();

            this.renderContainerDetail(containerDetail);
            this.showModal();
        } catch (error) {
            this.showNotification('Error loading container details: ' + error.message, 'error');
        }
    }

    renderContainerDetail(detail) {
        const container = detail.container;
        const stats = detail.stats;

        document.getElementById('modal-title').textContent =
            `${container.Name.substring(1)} Details`; // Remove leading /

        // Info tab
        const infoDiv = document.getElementById('container-info');
        infoDiv.innerHTML = `
            <div class="info-row">
                <span class="label">ID:</span>
                <span class="value"><code>${container.Id}</code></span>
            </div>
            <div class="info-row">
                <span class="label">Image:</span>
                <span class="value">${container.Config.Image}</span>
            </div>
            <div class="info-row">
                <span class="label">State:</span>
                <span class="value">${container.State.Status}</span>
            </div>
            <div class="info-row">
                <span class="label">Started:</span>
                <span class="value">${new Date(container.State.StartedAt).toLocaleString()}</span>
            </div>
            <div class="info-row">
                <span class="label">Platform:</span>
                <span class="value">${container.Platform}</span>
            </div>
            <div class="info-row">
                <span class="label">Path:</span>
                <span class="value"><code>${container.Path}</code></span>
            </div>
            <div class="info-row">
                <span class="label">Args:</span>
                <span class="value"><code>${container.Args.join(' ')}</code></span>
            </div>
            ${this.formatNetworkSettings(container.NetworkSettings)}
        `;

        // Load logs
        this.loadContainerLogs(container.Id);

        // Stats tab
        if (stats) {
            this.renderContainerStats(stats);
        } else {
            document.getElementById('container-stats').innerHTML =
                '<p>Stats not available (container may not be running)</p>';
        }
    }

    formatNetworkSettings(networkSettings) {
        if (!networkSettings || !networkSettings.Networks) return '';

        const networksHTML = Object.entries(networkSettings.Networks).map(([name, network]) => `
            <div class="info-row">
                <span class="label">Network (${name}):</span>
                <span class="value">${network.IPAddress || 'N/A'}</span>
            </div>
        `).join('');

        return networksHTML;
    }

    async loadContainerLogs(containerId) {
        try {
            const response = await fetch(`/api/containers/${containerId}/logs?tail=100`);
            const logs = await response.text();

            document.getElementById('logs-content').textContent = logs || 'No logs available';
        } catch (error) {
            document.getElementById('logs-content').textContent = 'Error loading logs: ' + error.message;
        }
    }

    renderContainerStats(stats) {
        const statsDiv = document.getElementById('container-stats');

        // Calculate CPU percentage
        const cpuPercent = this.calculateCPUPercent(stats);

        // Calculate memory usage
        const memoryUsage = stats.memory_stats.usage || 0;
        const memoryLimit = stats.memory_stats.limit || 0;
        const memoryPercent = memoryLimit > 0 ? (memoryUsage / memoryLimit) * 100 : 0;

        statsDiv.innerHTML = `
            <div class="info-row">
                <span class="label">CPU Usage:</span>
                <span class="value">${cpuPercent.toFixed(2)}%</span>
            </div>
            <div class="info-row">
                <span class="label">Memory Usage:</span>
                <span class="value">${this.formatBytes(memoryUsage)} / ${this.formatBytes(memoryLimit)} (${memoryPercent.toFixed(2)}%)</span>
            </div>
            <div class="info-row">
                <span class="label">Network RX:</span>
                <span class="value">${this.formatBytes(this.getNetworkStats(stats, 'rx_bytes'))}</span>
            </div>
            <div class="info-row">
                <span class="label">Network TX:</span>
                <span class="value">${this.formatBytes(this.getNetworkStats(stats, 'tx_bytes'))}</span>
            </div>
            <div class="info-row">
                <span class="label">Block I/O Read:</span>
                <span class="value">${this.formatBytes(this.getBlockIOStats(stats, 'read'))}</span>
            </div>
            <div class="info-row">
                <span class="label">Block I/O Write:</span>
                <span class="value">${this.formatBytes(this.getBlockIOStats(stats, 'write'))}</span>
            </div>
        `;
    }

    calculateCPUPercent(stats) {
        const cpuStats = stats.cpu_stats;
        const preCpuStats = stats.precpu_stats;

        if (!cpuStats || !preCpuStats) return 0;

        const cpuDelta = cpuStats.cpu_usage.total_usage - preCpuStats.cpu_usage.total_usage;
        const systemDelta = cpuStats.system_cpu_usage - preCpuStats.system_cpu_usage;

        if (systemDelta > 0 && cpuDelta > 0) {
            return (cpuDelta / systemDelta) * cpuStats.online_cpus * 100;
        }
        return 0;
    }

    getNetworkStats(stats, type) {
        if (!stats.networks) return 0;

        return Object.values(stats.networks).reduce((total, network) => {
            return total + (network[type] || 0);
        }, 0);
    }

    getBlockIOStats(stats, type) {
        if (!stats.blkio_stats || !stats.blkio_stats.io_service_bytes_recursive) return 0;

        const ioStats = stats.blkio_stats.io_service_bytes_recursive;
        const operation = type === 'read' ? 'Read' : 'Write';

        return ioStats.reduce((total, stat) => {
            return stat.op === operation ? total + stat.value : total;
        }, 0);
    }

    showModal() {
        document.getElementById('container-modal').classList.add('active');
        document.body.style.overflow = 'hidden';
    }

    closeModal() {
        document.getElementById('container-modal').classList.remove('active');
        document.body.style.overflow = 'auto';
    }

    // Utility functions
    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    showNotification(message, type = 'info') {
        // Create notification element
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.innerHTML = `
            <i class="fas fa-${type === 'success' ? 'check' : type === 'error' ? 'times' : 'info'}-circle"></i>
            <span>${message}</span>
        `;

        // Add styles if not already present
        if (!document.querySelector('.notification-styles')) {
            const style = document.createElement('style');
            style.className = 'notification-styles';
            style.textContent = `
                .notification {
                    position: fixed;
                    top: 20px;
                    right: 20px;
                    padding: 12px 20px;
                    border-radius: 6px;
                    color: white;
                    font-weight: 500;
                    z-index: 1001;
                    display: flex;
                    align-items: center;
                    gap: 8px;
                    max-width: 400px;
                    animation: slideIn 0.3s ease;
                }
                .notification-success { background: #10b981; }
                .notification-error { background: #ef4444; }
                .notification-info { background: #3b82f6; }
                @keyframes slideIn {
                    from { transform: translateX(100%); opacity: 0; }
                    to { transform: translateX(0); opacity: 1; }
                }
            `;
            document.head.appendChild(style);
        }

        document.body.appendChild(notification);

        // Remove after 5 seconds
        setTimeout(() => {
            notification.remove();
        }, 5000);
    }

    // ============ SYSTEMD SERVICES MANAGEMENT ============

    async loadServices() {
        try {
            const response = await fetch('/api/services');
            const services = await response.json();
            this.renderServicesTable(services);
        } catch (error) {
            console.error('Failed to load services:', error);
            this.showNotification('Failed to load services', 'error');
        }
    }

    renderServicesTable(services) {
        const tbody = document.querySelector('#services-table tbody');
        if (!tbody) return;

        if (!services || services.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6">No services found</td></tr>';
            return;
        }

        tbody.innerHTML = services.map(service => this.renderServiceRow(service)).join('');
    }

    renderServiceRow(service) {
        const statusClass = this.getServiceStatusClass(service.active_state);
        const actions = this.getServiceActions(service);

        return `
            <tr>
                <td>
                    <div class="service-name" onclick="dockerManager.showServiceDetail('${service.name}')" style="cursor: pointer;">
                        <strong>${service.name}</strong>
                        <br><small style="color: #666;">${service.unit}</small>
                    </div>
                </td>
                <td>
                    <span class="status-badge ${service.load_state.toLowerCase()}">${service.load_state}</span>
                </td>
                <td>
                    <span class="status-badge ${statusClass}">${service.active_state}</span>
                </td>
                <td>
                    <span class="sub-state">${service.sub_state}</span>
                </td>
                <td class="description" style="max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                    ${service.description || 'No description'}
                </td>
                <td class="actions">
                    ${actions}
                </td>
            </tr>
        `;
    }

    getServiceStatusClass(activeState) {
        switch (activeState) {
            case 'active': return 'running';
            case 'inactive': return 'stopped';
            case 'failed': return 'error';
            case 'activating': return 'starting';
            case 'deactivating': return 'stopping';
            default: return 'unknown';
        }
    }

    getServiceActions(service) {
        const isActive = service.active_state === 'active';
        const isLoaded = service.load_state === 'loaded';

        let actions = [];

        if (isLoaded) {
            if (isActive) {
                actions.push(`<button class="btn btn-sm btn-warning" onclick="dockerManager.stopService('${service.name}')">
                    <i class="fas fa-stop"></i>
                </button>`);
                actions.push(`<button class="btn btn-sm btn-secondary" onclick="dockerManager.restartService('${service.name}')">
                    <i class="fas fa-redo"></i>
                </button>`);
            } else {
                actions.push(`<button class="btn btn-sm btn-success" onclick="dockerManager.startService('${service.name}')">
                    <i class="fas fa-play"></i>
                </button>`);
            }

            actions.push(`<button class="btn btn-sm btn-primary" onclick="dockerManager.showServiceDetail('${service.name}')">
                <i class="fas fa-eye"></i>
            </button>`);
        }

        return actions.join(' ');
    }

    async startService(serviceName) {
        try {
            const response = await fetch(`/api/services/${serviceName}/start`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification(`Service ${serviceName} started successfully`, 'success');
                await this.loadServices();
            } else {
                const error = await response.text();
                this.showNotification(`Failed to start service: ${error}`, 'error');
            }
        } catch (error) {
            console.error('Failed to start service:', error);
            this.showNotification('Failed to start service', 'error');
        }
    }

    async stopService(serviceName) {
        try {
            const response = await fetch(`/api/services/${serviceName}/stop`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification(`Service ${serviceName} stopped successfully`, 'success');
                await this.loadServices();
            } else {
                const error = await response.text();
                this.showNotification(`Failed to stop service: ${error}`, 'error');
            }
        } catch (error) {
            console.error('Failed to stop service:', error);
            this.showNotification('Failed to stop service', 'error');
        }
    }

    async restartService(serviceName) {
        try {
            const response = await fetch(`/api/services/${serviceName}/restart`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification(`Service ${serviceName} restarted successfully`, 'success');
                await this.loadServices();
            } else {
                const error = await response.text();
                this.showNotification(`Failed to restart service: ${error}`, 'error');
            }
        } catch (error) {
            console.error('Failed to restart service:', error);
            this.showNotification('Failed to restart service', 'error');
        }
    }

    async enableService(serviceName) {
        try {
            const response = await fetch(`/api/services/${serviceName}/enable`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification(`Service ${serviceName} enabled successfully`, 'success');
                await this.loadServices();
            } else {
                const error = await response.text();
                this.showNotification(`Failed to enable service: ${error}`, 'error');
            }
        } catch (error) {
            console.error('Failed to enable service:', error);
            this.showNotification('Failed to enable service', 'error');
        }
    }

    async disableService(serviceName) {
        try {
            const response = await fetch(`/api/services/${serviceName}/disable`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showNotification(`Service ${serviceName} disabled successfully`, 'success');
                await this.loadServices();
            } else {
                const error = await response.text();
                this.showNotification(`Failed to disable service: ${error}`, 'error');
            }
        } catch (error) {
            console.error('Failed to disable service:', error);
            this.showNotification('Failed to disable service', 'error');
        }
    }

    async showServiceDetail(serviceName) {
        try {
            const response = await fetch(`/api/services/${serviceName}`);
            const detail = await response.json();
            this.renderServiceDetail(detail);
            this.showServiceModal();
        } catch (error) {
            console.error('Failed to load service detail:', error);
            this.showNotification('Failed to load service details', 'error');
        }
    }

    renderServiceDetail(detail) {
        const title = document.getElementById('service-modal-title');
        const serviceInfo = document.getElementById('service-info');
        const serviceLogs = document.getElementById('service-logs-content');
        const serviceStatus = document.getElementById('service-status-content');

        title.textContent = `Service: ${detail.service.name}`;

        // Render service info
        serviceInfo.innerHTML = `
            <div class="service-detail-grid" style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px;">
                <div class="detail-section">
                    <h4>Basic Information</h4>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Name:</span>
                        <span class="value">${detail.service.name}</span>
                    </div>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Unit:</span>
                        <span class="value">${detail.service.unit || 'N/A'}</span>
                    </div>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Type:</span>
                        <span class="value">${detail.service.type || 'N/A'}</span>
                    </div>
                    <div class="detail-row" style="margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Description:</span><br>
                        <span class="value" style="color: #666;">${detail.service.description || 'No description'}</span>
                    </div>
                </div>
                <div class="detail-section">
                    <h4>Status Information</h4>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Load State:</span>
                        <span class="value status-badge ${detail.service.load_state}">${detail.service.load_state}</span>
                    </div>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Active State:</span>
                        <span class="value status-badge ${this.getServiceStatusClass(detail.service.active_state)}">${detail.service.active_state}</span>
                    </div>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Sub State:</span>
                        <span class="value">${detail.service.sub_state}</span>
                    </div>
                    <div class="detail-row" style="display: flex; justify-content: space-between; margin: 8px 0;">
                        <span class="label" style="font-weight: bold;">Main PID:</span>
                        <span class="value">${detail.service.main_pid || 'N/A'}</span>
                    </div>
                </div>
                <div class="detail-section" style="grid-column: 1 / -1;">
                    <h4>Actions</h4>
                    <div class="service-actions" style="display: flex; gap: 10px; flex-wrap: wrap;">
                        ${this.getServiceDetailActions(detail.service)}
                    </div>
                </div>
            </div>
        `;

        // Render logs
        serviceLogs.textContent = detail.logs.join('\n');

        // Render status
        serviceStatus.textContent = detail.status;
    }

    getServiceDetailActions(service) {
        const isActive = service.active_state === 'active';
        const isLoaded = service.load_state === 'loaded';

        let actions = [];

        if (isLoaded) {
            if (isActive) {
                actions.push(`<button class="btn btn-warning" onclick="dockerManager.stopService('${service.name}'); dockerManager.closeServiceModal();">
                    <i class="fas fa-stop"></i> Stop Service
                </button>`);
                actions.push(`<button class="btn btn-secondary" onclick="dockerManager.restartService('${service.name}'); dockerManager.closeServiceModal();">
                    <i class="fas fa-redo"></i> Restart Service
                </button>`);
            } else {
                actions.push(`<button class="btn btn-success" onclick="dockerManager.startService('${service.name}'); dockerManager.closeServiceModal();">
                    <i class="fas fa-play"></i> Start Service
                </button>`);
            }

            actions.push(`<button class="btn btn-info" onclick="dockerManager.enableService('${service.name}');">
                <i class="fas fa-check"></i> Enable Service
            </button>`);
            actions.push(`<button class="btn btn-danger" onclick="dockerManager.disableService('${service.name}');">
                <i class="fas fa-times"></i> Disable Service
            </button>`);
        }

        return actions.join(' ');
    }

    showServiceModal() {
        const modal = document.getElementById('service-modal');
        modal.style.display = 'flex';

        // Setup event listeners for service modal
        this.setupServiceModalListeners();
    }

    closeServiceModal() {
        const modal = document.getElementById('service-modal');
        modal.style.display = 'none';
    }

    setupServiceModalListeners() {
        // Remove existing listeners to prevent duplicates
        const closeBtn = document.querySelector('.service-modal-close');
        if (closeBtn) {
            closeBtn.replaceWith(closeBtn.cloneNode(true));
            document.querySelector('.service-modal-close').addEventListener('click', () => {
                this.closeServiceModal();
            });
        }

        // Service detail tabs
        document.querySelectorAll('.service-tab-btn').forEach(btn => {
            btn.replaceWith(btn.cloneNode(true));
        });

        document.querySelectorAll('.service-tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.switchServiceDetailTab(btn.dataset.tab);
            });
        });
    }

    switchServiceDetailTab(tabName) {
        // Remove active class from all tabs and content
        document.querySelectorAll('.service-tab-btn').forEach(btn => btn.classList.remove('active'));
        document.querySelectorAll('.service-detail-tab').forEach(tab => tab.classList.remove('active'));

        // Add active class to selected tab and content
        document.querySelector(`.service-tab-btn[data-tab="${tabName}"]`).classList.add('active');
        document.getElementById(`service-${tabName}`).classList.add('active');
    }
}

// Initialize the application
const dockerManager = new DockerManager();
