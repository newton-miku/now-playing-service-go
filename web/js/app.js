new Vue({
    el: '#app',
    data: {
        lang: localStorage.getItem('lang') || 'zh',
        currentTab: 'main',
        showApiKey: false,
        statusData: {
            music: { status: 'None' },
            foreground: null
        },
        platforms: [],
        selectedPlatform: 'netease',
        preferredPlatform: 'netease',
        useGlobalDetect: true,
        lastUpdate: '-',
        appSettings: {
            preferred_platform: 'netease',
            port: '8080',
            check_interval_ms: 100,
            auto_open_browser: true,
            auto_start: false,
            smtc_preferred: true,
            enable_report: true,
            report_server_url: '',
            report_interval_ms: 1000,
            report_device_id: '',
            report_device_name: '',
            report_api_key: '',
            log_level: 1
        },
        showSaveStatus: false,
        // Log related
        logs: [],
        logFilter: 'ALL',
        logLevels: ['DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'],
        logEventSource: null,
        logConnected: false,
        versionInfo: {
            version: 'Loading...',
            buildTime: '-',
            goVersion: '-',
            platform: '-'
        }
    },
    computed: {
        t() {
            return i18n[this.lang];
        },
        filteredLogs() {
            if (this.logFilter === 'ALL') return this.logs;
            return this.logs.filter(l => l.level_name === this.logFilter);
        }
    },
    watch: {
        currentTab(newTab) {
            if (newTab === 'logs') {
                this.connectLogs();
            } else if (this.logEventSource) {
                this.logEventSource.close();
                this.logEventSource = null;
                this.logConnected = false;
            }
        }
    },
    mounted() {
        this.fetchPlatforms();
        this.fetchSettings();
        this.fetchVersion();
        this.refresh();
        setInterval(this.refresh, 1000);
    },
    methods: {
        toggleLang() {
            this.lang = this.lang === 'zh' ? 'en' : 'zh';
            localStorage.setItem('lang', this.lang);
        },
        fetchPlatforms() {
            axios.get('/api/platforms').then(res => {
                this.platforms = res.data.platforms;
            });
        },
        fetchSettings() {
            axios.get('/api/settings').then(res => {
                this.appSettings = res.data;
                this.preferredPlatform = res.data.preferred_platform;
            });
        },
        fetchVersion() {
            axios.get('/api/version').then(res => {
                this.versionInfo = res.data;
            }).catch(err => {
                console.error('Failed to fetch version:', err);
                this.versionInfo = {
                    version: 'Unknown',
                    buildTime: '-',
                    goVersion: '-',
                    platform: '-'
                };
            });
        },
        refresh() {
            const params = this.useGlobalDetect
                ? { preferred: this.preferredPlatform }
                : { platform: this.selectedPlatform };

            axios.get('/api/status', { params }).then(res => {
                this.statusData = res.data;
                this.lastUpdate = new Date().toLocaleTimeString();
            }).catch(err => console.error(err));
        },
        saveSettings() {
            // Validation
            if (this.appSettings.report_interval_ms < 100) {
                this.appSettings.report_interval_ms = 100;
            }
            if (this.appSettings.check_interval_ms < 100) {
                this.appSettings.check_interval_ms = 100;
            }

            axios.post('/api/settings', this.appSettings).then(res => {
                this.showSaveStatus = true;
                setTimeout(() => {
                    this.showSaveStatus = false;
                }, 2000);
            }).catch(err => console.error(err));
        },
        connectLogs() {
            if (this.logEventSource) return;

            this.logEventSource = new EventSource('/api/logs/stream');
            this.logEventSource.onopen = () => {
                this.logConnected = true;
            };
            this.logEventSource.onmessage = (event) => {
                try {
                    const entry = JSON.parse(event.data);
                    this.logs.push(entry);
                    if (this.logs.length > 500) this.logs.shift();

                    // Auto scroll to bottom
                    this.$nextTick(() => {
                        const list = document.getElementById('logList');
                        if (list) list.scrollTop = list.scrollHeight;
                    });
                } catch (e) {
                    console.error('Failed to parse log:', e);
                }
            };
            this.logEventSource.onerror = () => {
                this.logConnected = false;
                if (this.logEventSource) {
                    this.logEventSource.close();
                    this.logEventSource = null;
                }
                // Reconnect after 3s
                setTimeout(this.connectLogs, 3000);
            };
        },
        formatTime(ts) {
            return new Date(ts).toLocaleTimeString();
        },
        openExternal(url) {
            axios.post('/api/open-external', { url: url }).catch(err => {
                console.error('Failed to open external URL:', err);
                // Fallback: try to open in new tab
                window.open(url, '_blank');
            });
        }
    }
});
