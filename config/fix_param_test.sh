#!/bin/bash

# Fix parameter test errors by mapping to specific errors
sed -i '' 's/expectedError: ErrParametersInvalid,/expectedError: ErrKTooLow,/' config/parameters_test.go

# Fix specific test cases based on their names
sed -i '' '/name: "invalid AlphaPreference 1"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrAlphaPreferenceTooLow,/' config/parameters_test.go
sed -i '' '/name: "invalid AlphaPreference 2"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrAlphaPreferenceTooHigh,/' config/parameters_test.go
sed -i '' '/name: "invalid AlphaConfidence 1"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrAlphaConfidenceTooLow,/' config/parameters_test.go
sed -i '' '/name: "invalid AlphaConfidence 2"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrAlphaConfidenceTooHigh,/' config/parameters_test.go
sed -i '' '/name: "invalid AlphaConfidence 3"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrAlphaConfidenceTooSmall,/' config/parameters_test.go
sed -i '' '/name: "invalid Beta 1"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrBetaTooLow,/' config/parameters_test.go
sed -i '' '/name: "invalid Beta 2"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrBetaTooHigh,/' config/parameters_test.go
sed -i '' '/name: "invalid ConcurrentRepolls"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrConcurrentRepollsTooLow,/' config/parameters_test.go
sed -i '' '/name: "invalid OptimalProcessing"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrOptimalProcessingTooLow,/' config/parameters_test.go
sed -i '' '/name: "invalid MaxOutstandingItems"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrMaxOutstandingItemsTooLow,/' config/parameters_test.go
sed -i '' '/name: "invalid MaxItemProcessingTime"/,/expectedError:/ s/expectedError: ErrKTooLow,/expectedError: ErrMaxItemProcessingTimeTooLow,/' config/parameters_test.go