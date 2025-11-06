# Example 05: Python Client for Lux Bridge

This example demonstrates using the Lux bridge from Python via gRPC.

## What It Shows

- Python client connecting to Go bridge service
- Initiating cross-chain transfers from Python
- Monitoring transfer status
- Error handling and retries

## Setup

```bash
cd examples/05-python-client

# Create virtual environment
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r requirements.txt
```

## Run the Bridge Service

First, start the Go bridge gRPC server:

```bash
# In terminal 1
cd examples/05-python-client
go run server/main.go
```

## Run the Python Client

```bash
# In terminal 2  
python client.py
```

## Run the Tests

```bash
pytest test_client.py -v
```

## Expected Output

```
=== Python Bridge Client ===

Connecting to bridge service at localhost:50051...
✓ Connected successfully

Initiating transfer:
  Amount: 1.5 ETH
  From:   ethereum
  To:     lux

✓ Transfer initiated: tx_abc123...
✓ Monitoring status...

[Pending] 0/6 confirmations
[Validating] 3/6 confirmations
[Confirmed] 6/6 confirmations
✓ Transfer completed!

Transfer Details:
  ID:        tx_abc123...
  Amount:    1.5 ETH
  Status:    COMPLETED
  Duration:  3.2s
```

## Files

- `client.py` - Python client demonstrating bridge usage
- `test_client.py` - Pytest tests
- `server/main.go` - Go gRPC server wrapping the bridge
- `proto/bridge.proto` - Protocol buffer definitions
- `requirements.txt` - Python dependencies

## Key Concepts

This demonstrates:
1. **Language Interoperability**: Go backend, Python frontend
2. **gRPC Communication**: Efficient cross-language RPC
3. **Async Python**: Using asyncio for concurrent operations
4. **Error Handling**: Retries and timeouts

## Integration Pattern

```python
# 1. Create client
client = BridgeClient("localhost:50051")

# 2. Connect
await client.connect()

# 3. Initiate transfer
transfer_id = await client.transfer(
    asset="ETH",
    amount="1500000000000000000",
    source_chain="ethereum",
    dest_chain="lux",
)

# 4. Monitor status
status = await client.get_status(transfer_id)
```

## Play With It

Try modifying:
- Transfer amounts and chains
- Add more Python features
- Implement wallet integration
- Add transaction history
