import subprocess

def run_config(endpoint_dict):
    lines = ['vpn_mode = "general"', '', '[endpoint]']
    for k, v in endpoint_dict.items():
        if isinstance(v, str):
            if '\n' in v:
                lines.append(f'{k} = """\n{v}\n"""')
            else:
                lines.append(f'{k} = "{v}"')
        elif isinstance(v, bool):
            lines.append(f'{k} = {"true" if v else "false"}')
        elif isinstance(v, list):
            items = ", ".join([f'"{x}"' for x in v])
            lines.append(f'{k} = [{items}]')
        elif isinstance(v, int):
            lines.append(f'{k} = {v}')
            
    lines.extend(['', '[client]', 'socks5_port = 1088', 'http_port = 1089', 'forced_transport = "http2"', 'kill_switch = false'])
    content = "\n".join(lines)
    
    with open("scratch/temp_test.toml", "w") as f:
        f.write(content)
        
    try:
        res = subprocess.run(
            ["./bin/trusttunnel_client", "--config", "scratch/temp_test.toml"],
            capture_output=True, text=True, timeout=2
        )
        output = res.stderr + res.stdout
    except subprocess.TimeoutExpired as e:
        output = e.stderr + e.stdout if e.stderr else "TIMEOUT"
        
    return output

base_endpoint = {
    "hostname": "vpn.example.com",
    "addresses": ["127.0.0.1:443"],
    "username": "test-user",
    "password": "test-password",
    "upstream_protocol": "http2",
    "skip_verification": True,
}

# Suffix keys to test
suffix_keys = {
    "port": 443,
    "address": "127.0.0.1",
    "protocol": "http2",
    "prefix": "a0b0/f0f0",
    "verification": True,
    "random": "a0b0/f0f0",
    "certificate": "-----BEGIN CERTIFICATE-----\nMIIDFTCCAf2gAwIBAgIUTfhfRC7UppRit2j2n5hMfLwHhxowDQYJKoZIhvcNAQEL\nBQAwGjEYMBYGA1UEAwwPdnBuLmV4YW1wbGUuY29tMB4XDTI2MDYyMTEzMTA0OFoX\nDTI3MDYyMTEzMTA0OFowGjEYMBYGA1UEAwwPdnBuLmV4YW1wbGUuY29tMIIBIjAN\nBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA25zl0YjvRjBogEx8a2Pi59fcAscM\nfNy0R+a9JkuU4I4VOayJij4lg9FbgewdGAkf/SYfvyebtnIxwbPYu6nBexyiR4aQ\nGUtOc5WBUeM2lk2UvP/bjWPRSyVy9GUoB6Jx1I+rHS7CFYhIYqQ9lnGZDADfwjls\nhYwXTB45B0vz+FtrUa7okaJ+FZI45jl1I/pc77ZwZExOg1KVSmBIdvnXpEIXwLgF\nU+jzt//Kz7t/B4/buUTArOrEVsi/m/qSHRvvIdk5guERQ8Cvm4lMIu4fZ55h8UPg\n1j53psZUrscELCY8Sx+ffuuzAXYxyicL4rnpXkWBP+II4LwgzUHl7QtQfwIDAQAB\no1MwUTAdBgNVHQ4EFgQUnjeZyKA14G/MPjrim7nvYGMs4uwwHwYDVR0jBBgwFoAU\njeZyKA14G/MPjrim7nvYGMs4uwwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0B\nAQsFAAOCAQEAj6mslRal+6xuFKSFlIuqdBGZOoExY5HtpYzOnK/8Pf+NPxhHEG1b\nRQxcZ6L8e7TcPLElddJ/zH8liFpqRvwvxOJ3NtavhWyshPTRAIj5lDg42q1fSF2y\nFQrRiLXovf2mtw+erxYAqGkVkV7pxWYP7XV2zciID33GKEYpGGy4JD7ugMpQvqPh\nFv+Mb+lXLPoYVRpXg1IFZIuCpgiU4Be8zh5H92xet1W4EHPF4w4771AhQjA8T/Cm\nP+dJieDW44yHDkvJhp/JD85uJRT9oub12NgyGwxHiztXgHtiRg8FP7294+Mz5ZDr\n9BWz6ZYwlYwjw8g5zUZeUkf6jqCYSahGDg==\n-----END CERTIFICATE-----",
    "name": "primary",
}

# Test adding each one
for k, v in suffix_keys.items():
    temp = base_endpoint.copy()
    temp[k] = v
    out = run_config(temp)
    if "is not a table" not in out:
        print(f"Bypassed 'is not a table' by adding key {k}={v}!")
        print(f"Output: {out[:150]}")
